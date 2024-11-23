package models

type DAGsCountResponse struct {
	Status         string         `json:"status"`
	Total          int            `json:"total"`
	StatusCounts   map[string]int `json:"statusCounts"`
	SeverityCounts map[string]int `json:"severityCounts"`
}

func NewDAGsCountResponse(statusCounts, severityCounts map[string]int) (a DAGsCountResponse) {
	a = DAGsCountResponse{}
	a.Status = "ok"
	a.StatusCounts = statusCounts

	for _, count := range statusCounts {
		a.Total += count
	}

	return
}
