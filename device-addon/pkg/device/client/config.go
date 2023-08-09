package client

type MQTTBrokerInfo struct {
	Host      string `json:"host"`
	ClientId  string `json:"clientId"`
	Qos       int    `json:"qos"`
	KeepAlive uint16 `json:"keepAlive"`

	ConnEstablishingRetry int `json:"connEstablishingRetry"`

	// anonymous, basic or certificates (ca, cert and key)
	AuthMode      string `json:"authMode"`
	CredentialDir string `json:"credentialDir"`
}
