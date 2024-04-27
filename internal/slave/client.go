package slave

import (
	"github.com/duo/octopus/internal/common"
)

type Client interface {
	Vendor() string

	SendEvent(_ *common.OctopusEvent) (*common.OctopusEvent, error)

	Dispose()
}
