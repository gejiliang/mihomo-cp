package model

import "time"

type AppSettings struct {
	ID             int    `json:"id" db:"id"`
	MihomoConfig   string `json:"mihomo_config" db:"mihomo_config"`
	MihomoDir      string `json:"mihomo_dir" db:"mihomo_dir"`
	MihomoBinary   string `json:"mihomo_binary" db:"mihomo_binary"`
	ExtController  string `json:"ext_controller" db:"ext_controller"`
	ExtSecret      string `json:"ext_secret" db:"ext_secret"`
	RawConfigDraft string `json:"raw_config_draft" db:"raw_config_draft"`
	// Generic isolation underlay for the throwaway scoring mihomo: physical
	// egress interface (mihomo interface-name) and explicit DNS upstreams
	// (comma-separated mihomo nameserver syntax). Empty = safe defaults;
	// operators set deployment-specific values via the settings API.
	IsolationInterface string    `json:"isolation_interface" db:"isolation_interface"`
	IsolationDNS       string    `json:"isolation_dns" db:"isolation_dns"`
	UpdatedAt          time.Time `json:"updated_at" db:"updated_at"`
}
