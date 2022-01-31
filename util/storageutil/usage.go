package storageutil

import (
	"github.com/eluv-io/errors-go"
	"github.com/ricochet2200/go-disk-usage/du"

	"github.com/eluv-io/common-go/format/bytesize"
)

// GetUsage returns utilization for the storage volume at the given path.
func GetUsage(path string) (Usage, error) {
	usage := du.NewDiskUsage(path)
	if usage.Size() == 0 {
		// du.NewDiskUsage unfortunately ignores errors - if the total size of
		// the volume is 0, however, there was likely an error...
		return Usage{}, errors.NoTrace("storageutil.GetUsage", errors.K.IO, "path", path)
	}
	return Usage{
		Free:      bytesize.HR(usage.Free()),
		Available: bytesize.HR(usage.Available()),
		Capacity:  bytesize.HR(usage.Size()),
		Used:      bytesize.HR(usage.Used()),
		Usage:     float64(usage.Used()) / float64(usage.Size()),
	}, nil
}

// Usage is the utilization information of a storage volume.
type Usage struct {
	Free      bytesize.HR `json:"free"`
	Available bytesize.HR `json:"available"`
	Capacity  bytesize.HR `json:"capacity"`
	Used      bytesize.HR `json:"used"`
	Usage     float64     `json:"usage"` // as fraction Used / Capacity
}
