package common

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultPageSize    = 10
	defaultSendTimeout = 3 * time.Minute
)

type ArchiveChat struct {
	Vendor string `yaml:"vendor"`
	UID    string `yaml:"uid"`
	ChatID int64  `yaml:"chat_id"`
}

type Configure struct {
	Master struct {
		APIURL    string        `yaml:"api_url"`
		LocalMode bool          `yaml:"local_mode"`
		AdminID   int64         `yaml:"admin_id"`
		Token     string        `yaml:"token"`
		Proxy     string        `yaml:"proxy"`
		PageSize  int           `yaml:"page_size"`
		Archive   []ArchiveChat `yaml:"archive"`

		Telegraph struct {
			Enable bool     `ymal:"enable"`
			Proxy  string   `yaml:"proxy"`
			Tokens []string `yaml:"tokens"`
		} `yaml:"telegraph"`
	} `yaml:"master"`

	Service struct {
		Addr        string        `yaml:"addr"`
		Secret      string        `yaml:"secret"`
		SendTiemout time.Duration `yaml:"send_timeout"`
	} `yaml:"service"`

	Log struct {
		Level string `yaml:"level"`
	} `yaml:"log"`
}

func LoadConfig(path string) (*Configure, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	config := &Configure{}
	config.Master.APIURL = "https://api.telegram.org"
	config.Master.PageSize = defaultPageSize
	config.Service.SendTiemout = defaultSendTimeout
	if err := yaml.Unmarshal(file, &config); err != nil {
		return nil, err
	}

	return config, nil
}
