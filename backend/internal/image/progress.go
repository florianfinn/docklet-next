// Source: docklet-next/docs/design/images-updates.md
// Revision: f6033a46c9ef4ad490661addd7dac41ab0ed5a45
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Structured daemon pull progress whitelist for issue #229.

package image

import (
	"encoding/json"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"io"
	"regexp"
)

var pullLayerID = regexp.MustCompile(`^[a-f0-9]{12,64}$`)
var pullStatuses = map[string]bool{
	"Pulling fs layer": true, "Waiting": true, "Downloading": true, "Verifying Checksum": true,
	"Download complete": true, "Extracting": true, "Pull complete": true, "Already exists": true,
}

type safePullMessage = imageDTO.PullProgressFrame
type pullProgressCounts = imageDTO.PullProgressCounts

func writeSafePullProgress(reader io.Reader, writer io.Writer) error {
	decoder := json.NewDecoder(reader)
	for {
		var message struct {
			Status         string              `json:"status"`
			ID             string              `json:"id"`
			ProgressDetail *pullProgressCounts `json:"progressDetail"`
			Error          string              `json:"error"`
			ErrorDetail    json.RawMessage     `json:"errorDetail"`
		}
		err := decoder.Decode(&message)
		if err == io.EOF {
			return nil
		}
		if err != nil || message.Error != "" || (len(message.ErrorDetail) > 0 && string(message.ErrorDetail) != "null") {
			return imageDTO.Failure("image.pull-failed", 502)
		}
		safe := safePullMessage{Status: "Pulling image"}
		if pullStatuses[message.Status] {
			safe.Status = message.Status
		}
		if pullLayerID.MatchString(message.ID) {
			safe.ID = message.ID
		}
		if p := message.ProgressDetail; p != nil && p.Current >= 0 && p.Total >= p.Current {
			safe.ProgressDetail = p
		}
		if writer != nil {
			if err := json.NewEncoder(writer).Encode(safe); err != nil {
				return imageDTO.Failure("image.pull-failed", 502)
			}
		}
	}
}
