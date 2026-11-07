package domain

type VaultInfo struct {
	RootPath     string `json:"rootPath"`
	Name         string `json:"name"`
	LastOpenedAt string `json:"lastOpenedAt,omitempty"` // RFC3339
}

type TreeNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"` // vault-relative, normalized with /
	AbsPath  string      `json:"absPath"`
	IsDir    bool        `json:"isDir"`
	ModTime  string      `json:"modTime,omitempty"` // RFC3339
	Size     int64       `json:"size,omitempty"`
	Children []*TreeNode `json:"children,omitempty"`
}

type NoteDocument struct {
	Path        string `json:"path"` // vault-relative
	AbsPath     string `json:"absPath"`
	Content     string `json:"content"`
	MtimeNs     int64  `json:"mtimeNs"`
	SizeBytes   int64  `json:"sizeBytes"`
	ContentHash string `json:"contentHash"`
	LineEnding  string `json:"lineEnding"` // "\n" or "\r\n"
}

type ConflictKind string

const (
	ConflictExternalChange ConflictKind = "external_change"
)

type SaveResult struct {
	Saved        bool          `json:"saved"`
	Conflict     bool          `json:"conflict"`
	ConflictKind ConflictKind  `json:"conflictKind,omitempty"`
	Message      string        `json:"message,omitempty"`
	Document     *NoteDocument `json:"document,omitempty"`
}

type SearchOptions struct {
	Limit int `json:"limit"`
}

type SearchResult struct {
	Path    string  `json:"path"`
	Title   string  `json:"title"`
	MtimeNs int64   `json:"mtimeNs"`
	Snippet string  `json:"snippet"`
	Rank    float64 `json:"rank,omitempty"`
}

type Settings struct {
	RecentVaults    []string `json:"recentVaults"`
	LastActiveVault string   `json:"lastActiveVault,omitempty"`
	AutosaveMs      int      `json:"autosaveMs"`
	PreviewDefault  bool     `json:"previewDefault"`
	ConfigVersion   int      `json:"configVersion"`
}

type IndexDocument struct {
	Path        string   `json:"path"`
	AbsPath     string   `json:"absPath"`
	Title       string   `json:"title"`
	Body        string   `json:"body"`
	Tags        []string `json:"tags"`
	Aliases     []string `json:"aliases"`
	MtimeNs     int64    `json:"mtimeNs"`
	SizeBytes   int64    `json:"sizeBytes"`
	ContentHash string   `json:"contentHash"`
}
