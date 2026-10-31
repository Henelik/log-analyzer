package db

type FileStatus int16

const (
	FileStatusInvalid = iota
	FileStatusUplodaded
	FileStatusParsed
	FileStatusAnalysisInProgress
	FileStatusAnalyzed
)

// File represents a user-uploaded log file in the database
type File struct {
	Model

	// UploadedBy     uint       `gorm:"not null"`
	Filename       string     `gorm:"not null"`
	Headers        string     `gorm:"not null"`
	Lines          int32      `gorm:"not null"`
	AcceptedLines  int32      `gorm:"not null"`
	RejectedLines  int32      `gorm:"not null"`
	DuplicateLines int32      `gorm:"not null"`
	Status         FileStatus `gorm:"not null"`
}

type Row struct {
	Model
}

type Analysis struct {
	Model
}
