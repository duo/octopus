package channel

import "github.com/duo/octopus/internal/common"

type Filter interface {
	Process(in *common.OctopusEvent) *common.OctopusEvent
}

func ProcessFilter(f Filter, in *common.OctopusEvent) *common.OctopusEvent {
	return f.Process(in)
}
