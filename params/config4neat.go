package params

var GenCfg = GeneralConfig{PerfTest: false}

type GeneralConfig struct {
	PerfTest bool `json:"perfTest,omitempty"`
}
