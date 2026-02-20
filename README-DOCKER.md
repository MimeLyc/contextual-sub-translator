# Docker 验证指南

## 快速开始

1. **复制环境配置**:
   ```bash
   cp .env.example .env
   # 编辑 .env 文件，填写你的 LLM_API_KEY
   ```

2. **运行验证脚本**:
   ```bash
   ./verify-docker.sh
   ```

3. **手动测试**:
   ```bash
   # 构建镜像
   docker-compose build
   
   # 检查环境
   docker-compose run --rm ctxtrans env | grep -E "^(LLM_|SEARCH_|AGENT_|LOG_LEVEL|CRON_EXPR|MOVIE_DIR|ANIMATION_DIR|TELEPLAY_DIR|SHOW_DIR|DOCUMENTARY_DIR|PUID|PGID|TZ|ZONE)="
   
   # 检查ffmpeg
   docker-compose run --rm ctxtrans ffmpeg -version
   docker-compose run --rm ctxtrans ffprobe -version
   docker-compose run --rm ctxtrans sh -c "ffmpeg -hide_banner -decoders | grep -i libaribb24"
   
   # 运行服务
   docker-compose up -d
   docker-compose logs -f

   # 访问 Web UI
   open http://localhost:8080
   ```

## 目录结构

```
./test/           # 测试目录映射到容器内对应目录
├── movies/      → /movies
├── animations/  → /animations
├── teleplays/   → /teleplays
├── shows/       → /shows
└── documentaries/ → /documentaries
```

## 使用自定义目录

编辑 `docker-compose.yml`，将 `./test/movies` 等路径替换为你实际的媒体目录路径。

```yaml
volumes:
  - /path/to/your/movies:/movies
  - /path/to/your/animations:/animations
  # ... 以此类推
```

## 验证步骤

1. ✅ 镜像构建成功
2. ✅ 基础命令可用 (ffmpeg, ffprobe)
3. ✅ `libaribb24` 解码器可用（`arib_caption`）
4. ✅ 环境变量正确设置
5. ✅ 目录结构正确挂载
6. ✅ 服务可以正常启动 (需要API密钥)
7. ✅ Web UI 可访问 (`http://localhost:8080`)
