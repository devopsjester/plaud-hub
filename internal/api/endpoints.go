// Package api provides a Go client for the Plaud AI API.
//
// This is based on the unofficial reverse-engineered API documented at
// https://github.com/arbuzmell/plaud-api.
package api

const (
	apiBase = "https://api.plaud.ai"

	// Recordings
	endpointFileSimple = apiBase + "/file/simple/web"
	endpointFileList   = apiBase + "/file/list"

	// Tags
	endpointFileTag = apiBase + "/filetag/"
)
