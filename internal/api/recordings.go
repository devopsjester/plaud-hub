package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// ListRecordingsPage fetches one page of recordings sorted by start_time descending.
func (c *Client) ListRecordingsPage(ctx context.Context, skip, limit int) ([]Recording, error) {
	params := map[string]string{
		"skip":    strconv.Itoa(skip),
		"limit":   strconv.Itoa(limit),
		"is_trash": "0",
		"sort_by": "start_time",
		"is_desc": "true",
	}

	data, err := c.get(ctx, endpointFileSimple, params)
	if err != nil {
		return nil, fmt.Errorf("list recordings: %w", err)
	}

	var resp fileSimpleResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode recordings response: %w", err)
	}

	return resp.DataFileList, nil
}

// ListRecordingsInRange fetches all recordings within the given date range
// using client-side filtering with early-exit pagination.
func (c *Client) ListRecordingsInRange(ctx context.Context, from, to time.Time) ([]Recording, error) {
	const pageSize = 50
	var result []Recording

	for skip := 0; ; skip += pageSize {
		page, err := c.ListRecordingsPage(ctx, skip, pageSize)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}

		pastRange := false
		for _, rec := range page {
			created := rec.CreatedAt()
			if created.IsZero() {
				continue
			}
			// Skip recordings newer than "to" date.
			if !to.IsZero() && created.After(to) {
				continue
			}
			// Stop if we've gone past the "from" date (results are descending).
			if !from.IsZero() && created.Before(from) {
				pastRange = true
				break
			}
			result = append(result, rec)
		}

		if pastRange || len(page) < pageSize {
			break
		}
	}

	return result, nil
}

// GetRecordingDetails fetches full details (including trans_result and ai_content)
// for a batch of recording IDs via POST /file/list.
func (c *Client) GetRecordingDetails(ctx context.Context, fileIDs []string) ([]Recording, error) {
	if len(fileIDs) == 0 {
		return nil, nil
	}

	data, err := c.post(ctx, endpointFileList, fileIDs)
	if err != nil {
		return nil, fmt.Errorf("get recording details: %w", err)
	}

	var resp fileListResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode file list response: %w", err)
	}

	return resp.DataFileList, nil
}
