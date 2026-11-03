// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original bounded owning-host history sample contract.

package history

import "strings"

const SampleLimit = 4 * 1024 * 1024
const SamplePath = "/api/environments/0/history/sample"

func IsSampleRoute(method, path string) bool {
	return method == "GET" && strings.SplitN(path, "?", 2)[0] == SamplePath
}

type HostSample struct {
	CPUBusy     float64 `json:"cpuBusy"`
	CPUTotal    float64 `json:"cpuTotal"`
	MemoryUsed  float64 `json:"memoryUsed"`
	MemoryTotal float64 `json:"memoryTotal"`
	DiskUsed    float64 `json:"diskUsed"`
	DiskTotal   float64 `json:"diskTotal"`
	DiskSource  string  `json:"diskSource"`
	NetworkRx   float64 `json:"networkRx"`
	NetworkTx   float64 `json:"networkTx"`
}
type Counters struct {
	CPUTotal    float64 `json:"cpuTotal"`
	CPUSystem   float64 `json:"cpuSystem"`
	MemoryUsage float64 `json:"memoryUsage"`
	MemoryLimit float64 `json:"memoryLimit"`
	NetworkRx   float64 `json:"networkRx"`
	NetworkTx   float64 `json:"networkTx"`
	BlockRead   float64 `json:"blockRead"`
	BlockWrite  float64 `json:"blockWrite"`
}
type ContainerSample struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	ProjectID    string    `json:"projectId"`
	ProjectName  string    `json:"projectName"`
	Service      string    `json:"service"`
	State        string    `json:"state" enum:"running,restarting,paused,exited,created,dead,removing"`
	Health       string    `json:"health" enum:",none,starting,healthy,unhealthy"`
	ExitCode     int       `json:"exitCode"`
	OOMKilled    bool      `json:"oomKilled"`
	RestartCount int       `json:"restartCount"`
	StartedAt    string    `json:"startedAt"`
	FinishedAt   string    `json:"finishedAt"`
	StopIntent   bool      `json:"stopIntent"`
	StatsMissing bool      `json:"statsMissing"`
	Stats        *Counters `json:"stats,omitempty"`
}
type ProjectSample struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}
type Sample struct {
	Host       HostSample        `json:"host"`
	Containers []ContainerSample `json:"containers"`
	Projects   []ProjectSample   `json:"projects"`
}
type Problem struct {
	Code   string `json:"code" enum:"history.sample-too-large,history.sample-invalid,history.unavailable,history.invalid-range"`
	Status int    `json:"status"`
}

func (p *Problem) Error() string         { return p.Code }
func (p *Problem) GetStatus() int        { return p.Status }
func Fail(code string, status int) error { return &Problem{Code: code, Status: status} }
