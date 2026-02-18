package library

type SourceConfig struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type Source struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	ItemCount int    `json:"item_count"`
}

type Item struct {
	ID           string `json:"id"`
	SourceID     string `json:"source_id"`
	Name         string `json:"name"`
	Path         string `json:"path"`
	EpisodeCount int    `json:"episode_count"`
}

type SubtitleStatus struct {
	HasSourceSubtitle         bool     `json:"has_source_subtitle"`
	HasTargetSubtitle         bool     `json:"has_target_subtitle"`
	HasEmbeddedSubtitle       bool     `json:"has_embedded_subtitle"`
	HasEmbeddedTargetSubtitle bool     `json:"has_embedded_target_subtitle"`
	SourceSubtitleFiles       []string `json:"source_subtitle_files"`
	TargetSubtitleFiles       []string `json:"target_subtitle_files"`
	Languages                 []string `json:"languages"`
}

type Episode struct {
	ID           string         `json:"id"`
	SourceID     string         `json:"source_id"`
	ItemID       string         `json:"item_id"`
	Name         string         `json:"name"`
	Season       string         `json:"season"`
	MediaPath    string         `json:"media_path"`
	Subtitles    SubtitleStatus `json:"subtitles"`
	Translatable bool           `json:"translatable"`
}

type Library struct {
	Sources  []Source  `json:"sources"`
	Items    []Item    `json:"items"`
	Episodes []Episode `json:"episodes"`
}
