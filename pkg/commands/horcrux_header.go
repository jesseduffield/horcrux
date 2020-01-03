package commands

type horcruxHeader struct {
	OriginalFilename string `json:"originalFilename"`
	Timestamp        int64  `json:"timestamp"`
	Index            int    `json:"index"`
	Total            int    `json:"total"`
	Threshold        int    `json:"threshold"`
	KeyFragment      []byte `json:"keyFragment"`
}
