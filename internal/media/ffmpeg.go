package media

import (
	"bufio"
	"encoding/json"
	"os/exec"
	"path/filepath"

	"github.com/MimeLyc/contextual-sub-translator/internal/subtitle"
	"github.com/MimeLyc/contextual-sub-translator/pkg/file"
	"github.com/MimeLyc/contextual-sub-translator/pkg/log"
	"golang.org/x/text/language"
)

type ffmpeg struct {
	ffmpegCmd  string
	ffprobeCmd string
	filePath   string
	fileDir    string
	fileName   string
}

func NewFfmpeg(
	mediaPath string,
) ffmpeg {
	mediaPath, _ = filepath.Abs(mediaPath)

	// deal with media path
	mediaPath = filepath.Clean(mediaPath)
	mediaDir := filepath.Dir(mediaPath)
	mediaName := filepath.Base(mediaPath)

	return ffmpeg{
		ffmpegCmd:  "ffmpeg",
		ffprobeCmd: "ffprobe",
		filePath:   mediaPath,
		fileDir:    mediaDir,
		fileName:   mediaName,
	}
}

// Extract subtitle from media file and save to target path
func (ff ffmpeg) ExtractSubtitle(
	toDir string,
	name string,
) (string, error) {
	output := filepath.Join(toDir, name)

	cmdPath, err := exec.LookPath(ff.ffmpegCmd)
	if err != nil {
		return "", err
	}
	cmd := exec.Command(cmdPath, ff.extractSubArgs(output)...)

	// Redirect stderr to stdout
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Error("Failed to redirect stderr to stdout: %v", err)
	}
	cmd.Stderr = cmd.Stdout

	err = cmd.Start()
	if err != nil {
		log.Error("Execution failed: %v", err)
	}
	done := make(chan struct{})

	go func() {
		scanner := bufio.NewScanner(stdout)
		buf := make([]byte, 0, 10*1024*1024)
		scanner.Buffer(buf, 100*1024*1024)
		for scanner.Scan() {
			log.Info(scanner.Text())
		}
		if err := scanner.Err(); err != nil && err.Error() != "read |0: file already closed" {
			log.Error("Execution failed: %v", err)
		}
		close(done)
	}()

	err = cmd.Wait()
	<-done
	if err != nil {
		log.Error("Execution failed: %v", err)
	}
	return output, err
}

// Extract subtitle from media file and save to target path
func (ff ffmpeg) DefExtractSubtitle() (string, error) {
	return ff.ExtractSubtitle(
		ff.fileDir,
		// TODO
		file.ReplaceExt(ff.fileName, "ctxtrans.srt"))

}

func (ff ffmpeg) ReadSubtitleDescription() (subtitle.Descriptions, error) {
	cmdPath, err := exec.LookPath(ff.ffprobeCmd)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(cmdPath, ff.readProbeArgs(ff.filePath)...)

	output, err := cmd.Output()
	if err != nil {
		log.Error("Failed to run ffprobe: %v", err)
		return nil, err
	}

	var probeResult struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			Tags      struct {
				Language string `json:"language"`
				Title    string `json:"title"`
			} `json:"tags"`
			Disposition struct {
				Default int `json:"default"`
			} `json:"disposition"`
		} `json:"streams"`
	}

	if err := json.Unmarshal(output, &probeResult); err != nil {
		log.Error("Failed to parse ffprobe output: %v", err)
		return nil, err
	}

	descriptions := make([]subtitle.Description, 0)
	for _, stream := range probeResult.Streams {
		if stream.CodecType == "subtitle" {
			desc := subtitle.Description{
				Language:    stream.Tags.Language,
				SubLanguage: stream.Tags.Title,
				LangTag:     language.All.Make(stream.Tags.Language),
			}
			if desc.Language == "" {
				desc.Language = "und" // undefined
				desc.LangTag = language.Und
			}
			descriptions = append(descriptions, desc)
		}
	}

	return descriptions, nil
}

func (ffmpeg) readProbeArgs(path string) []string {
	return []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-select_streams",
		"s",
		path,
	}
}

func (f ffmpeg) extractSubArgs(targetPath string) []string {
	return []string{
		"-i", f.filePath,
		"-map", "0:s:0", // select first subtitle
		"-c:s", "srt", // convert to srt
		"-f", "srt", // output format
		targetPath,
	}
}
