package model

type License struct {
	Validity bool   `json:"validity" desc:"是否有效"`
	Trial    bool   `json:"trial" desc:"是否试用"`
	Message  string `json:"message"`
}

type Signature struct {
	License       *License `json:"license"`
	SignatureText []byte   `json:"signatureText"`
}
