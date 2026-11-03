// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original history, capacity and immutable condition read contracts.

package history

import "time"

type State string

const (
	Running   State = "running"
	Unhealthy State = "unhealthy"
	Down      State = "down"
	Stopped   State = "stopped"
	NoData    State = "nodata"
)
const (
	EventDown         = "container.condition.down"
	EventUnhealthy    = "container.condition.unhealthy"
	EventRestartLoop  = "container.condition.restart-loop"
	EventRunningAgain = "container.condition.running-again"
)

type Minutes struct {
	Running   int `json:"running"`
	Unhealthy int `json:"unhealthy"`
	Down      int `json:"down"`
	Stopped   int `json:"stopped"`
}
type Bucket struct {
	Start         time.Time `json:"start"`
	Partial       bool      `json:"partial"`
	State         State     `json:"state" enum:"running,unhealthy,down,stopped,nodata"`
	Minutes       Minutes   `json:"minutes"`
	CPUAverage    *float64  `json:"cpuAverage"`
	CPUMax        *float64  `json:"cpuMax"`
	MemoryAverage *float64  `json:"memoryAverage"`
	MemoryMax     *float64  `json:"memoryMax"`
	MemoryLimit   float64   `json:"memoryLimit"`
	NetworkRx     float64   `json:"networkRx"`
	NetworkTx     float64   `json:"networkTx"`
	BlockRead     float64   `json:"blockRead"`
	BlockWrite    float64   `json:"blockWrite"`
	DiskUsed      float64   `json:"diskUsed"`
	DiskTotal     float64   `json:"diskTotal"`
	DiskSource    string    `json:"diskSource"`
	ExitCode      int       `json:"exitCode"`
}
type Response struct {
	Range         string   `json:"range" enum:"24h,7d,30d"`
	Buckets       []Bucket `json:"buckets"`
	Totals        Minutes  `json:"totals"`
	UptimePercent *float64 `json:"uptimePercent"`
}
type Condition struct {
	Kind        string    `json:"kind" enum:"container,stack"`
	ContainerID string    `json:"containerId"`
	Name        string    `json:"name"`
	ProjectID   string    `json:"projectId"`
	Condition   State     `json:"condition" enum:"running,unhealthy,down,stopped,nodata"`
	Reason      string    `json:"reason" enum:",restart-loop,health,paused,intent,never-started,oom,exit,operator-stop"`
	ExitCode    int       `json:"exitCode"`
	Restarts10m int       `json:"restarts10m"`
	Since       time.Time `json:"since"`
	SampledAt   time.Time `json:"sampledAt"`
}
type Conditions struct {
	HostID    string      `json:"hostId"`
	SampledAt time.Time   `json:"sampledAt"`
	Items     []Condition `json:"items"`
}
type Capacity struct {
	HostID            string   `json:"hostId"`
	DiskSource        string   `json:"diskSource"`
	UsedBytes         float64  `json:"usedBytes"`
	TotalBytes        float64  `json:"totalBytes"`
	GrowthBytesPerDay *float64 `json:"growthBytesPerDay"`
	DaysUntilFull     *float64 `json:"daysUntilFull"`
	GrowthUnavailable bool     `json:"growthUnavailable"`
	History           Response `json:"history"`
}
