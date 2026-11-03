// Source: docklet-next/issues/338
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original condition-event and sustained-load metadata vocabulary.

package history

type ConditionEventMetadata struct {
	HostID        string `json:"hostId"`
	ContainerName string `json:"containerName"`
	ExitCode      int    `json:"exitCode"`
	Restarts      int    `json:"restarts"`
}

func (m ConditionEventMetadata) Fields() map[string]any {
	return map[string]any{"hostId": m.HostID, "containerName": m.ContainerName, "exitCode": m.ExitCode, "restarts": m.Restarts}
}

type LoadAlertMetadata struct {
	Metric    string  `json:"metric" enum:"cpu,memory"`
	Percent   float64 `json:"percent"`
	Threshold float64 `json:"threshold"`
	Minutes   int     `json:"minutes"`
	Host      string  `json:"host"`
	Container string  `json:"container"`
}

func (m LoadAlertMetadata) Fields() map[string]any {
	return map[string]any{"metric": m.Metric, "percent": m.Percent, "threshold": m.Threshold, "minutes": m.Minutes, "host": m.Host, "container": m.Container}
}
