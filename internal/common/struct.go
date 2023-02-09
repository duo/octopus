package common

import (
	"errors"
	"fmt"
	"strings"
)

type Limb struct {
	Type   string // Type: telegram, qq, wechat, etc
	UID    string
	ChatID string
}

func (l Limb) String() string {
	return fmt.Sprintf("%s%s%s%s%s", l.Type, VENDOR_SEP, l.UID, VENDOR_SEP, l.ChatID)
}

func LimbFromString(str string) (*Limb, error) {
	parts := strings.Split(str, VENDOR_SEP)
	if len(parts) != 3 {
		return nil, errors.New("limb format invalid")
	}

	return &Limb{parts[0], parts[1], parts[2]}, nil
}
