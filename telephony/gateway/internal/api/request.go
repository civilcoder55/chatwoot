package api

type InitiateCallRequest struct {
	To       string `json:"to"`
	From     string `json:"from"`
	SDPOffer string `json:"sdp_offer"`
}

type AcceptCallRequest struct {
	SDPAnswer string `json:"sdp_answer"`
}
