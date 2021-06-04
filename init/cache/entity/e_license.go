package entity

// UseLicense 已用授权
type UseLicense struct {
	PointCounts int `json:"pointCounts"`
	UserCounts  int `json:"userCounts"`
}

// UnUseLicense 剩余授权
type UnUseLicense struct {
	Days        int `json:"days"`
	PointCounts int `json:"pointCounts"`
	UserCounts  int `json:"userCounts"`
}

type UseLicenseResponse struct {
	Index      int
	UseLicense *UseLicense
	Err        error
}
