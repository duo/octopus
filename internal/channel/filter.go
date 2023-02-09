package channel

import "github.com/duo/octopus/internal/common"

type Filter interface {
	Process(in *common.OctopusEvent) (*common.OctopusEvent, bool)
}

// it returns modified evnet and a flag (false means drop)
func ProcessFilter(f Filter, in *common.OctopusEvent) (*common.OctopusEvent, bool) {
	return f.Process(in)
}
