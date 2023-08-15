package opcua

const (
	Endpoint = "endpoint"
)

const NODE = "nodeId"

type Config struct {
	SecurityPolicy string `json:"securityPolicy"` // Security policy: None, Basic128Rsa15, Basic256, Basic256Sha256
	SecurityMode   string `json:"securityMode"`   // Security mode: None, Sign, SignAndEncrypt
	CertFile       string `json:"certFile"`
	KeyFile        string `json:"keyFile"`
}
