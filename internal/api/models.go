package api

import (
	"encoding/json"
	"fmt"
	"time"
)

// Recording represents a Plaud recording (file) from the API.
type Recording struct {
	ID               string `json:"id"`
	Filename         string `json:"filename"`
	DurationMS       int64  `json:"duration"`
	Filesize         int64  `json:"filesize"`
	StartTime        int64  `json:"start_time"`
	HasTranscription bool   `json:"-"`
	HasSummary       bool   `json:"-"`

	// Raw fields used for transcript/summary extraction.
	TransResult json.RawMessage `json:"trans_result"`
	AIContent   string          `json:"ai_content"`
	SummaryList []string        `json:"summary_list"`
}

// CreatedAt returns the recording's creation time derived from StartTime (epoch ms).
func (r Recording) CreatedAt() time.Time {
	if r.StartTime <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(r.StartTime)
}

// DurationDisplay returns a human-readable "m:ss" duration string.
func (r Recording) DurationDisplay() string {
	totalSec := r.DurationMS / 1000
	min := totalSec / 60
	sec := totalSec % 60
	return fmt.Sprintf("%d:%02d", min, sec)
}

// TranscriptSegment represents a single segment of a transcription.
type TranscriptSegment struct {
	Speaker    string `json:"speaker"`
	Content    string `json:"content"`
	StartTime  int64  `json:"start_time"`
	EndTime    int64  `json:"end_time"`
}

// FormatTimestamp returns a "HH:MM:SS" timestamp for the segment's start time.
func (s TranscriptSegment) FormatTimestamp() string {
	total := s.StartTime / 1000
	h := total / 3600
	m := (total % 3600) / 60
	sec := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%02d:%02d", m, sec)
}

// fileListResponse is the JSON envelope returned by POST /file/list.
type fileListResponse struct {
	DataFileList []Recording `json:"data_file_list"`
}

// fileSimpleResponse is the JSON envelope returned by GET /file/simple/web.
type fileSimpleResponse struct {
	DataFileList []Recording `json:"data_file_list"`
}
