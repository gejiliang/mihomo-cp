package model

type MihomoConfig struct {
	Port               int                       `yaml:"port,omitempty"`
	SocksPort          int                       `yaml:"socks-port,omitempty"`
	MixedPort          int                       `yaml:"mixed-port,omitempty"`
	AllowLan           bool                      `yaml:"allow-lan,omitempty"`
	BindAddress        string                    `yaml:"bind-address,omitempty"`
	Mode               string                    `yaml:"mode,omitempty"`
	LogLevel           string                    `yaml:"log-level,omitempty"`
	IPv6               bool                      `yaml:"ipv6,omitempty"`
	ExternalController string                    `yaml:"external-controller,omitempty"`
	Secret             string                    `yaml:"secret,omitempty"`
	DNS                map[string]any            `yaml:"dns,omitempty"`
	Tun                map[string]any            `yaml:"tun,omitempty"`
	Listeners          []map[string]any          `yaml:"listeners,omitempty"`
	Extra              map[string]any            `yaml:",inline"`
	Proxies            []map[string]any          `yaml:"proxies,omitempty"`
	ProxyGroups        []map[string]any          `yaml:"proxy-groups,omitempty"`
	Rules              []string                  `yaml:"rules,omitempty"`
	RuleProviders      map[string]map[string]any `yaml:"rule-providers,omitempty"`
}
