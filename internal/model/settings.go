package model

import "time"

type AppSettings struct {
	ID            int       `json:"id" db:"id"`
	MihomoConfig  string    `json:"mihomo_config" db:"mihomo_config"`
	MihomoDir     string    `json:"mihomo_dir" db:"mihomo_dir"`
	MihomoBinary  string    `json:"mihomo_binary" db:"mihomo_binary"`
	ExtController string    `json:"ext_controller" db:"ext_controller"`
	ExtSecret     string    `json:"ext_secret" db:"ext_secret"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
}
