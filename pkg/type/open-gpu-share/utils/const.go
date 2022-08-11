package utils

const (
	ResourceName = "alibabacloud.com/gpu-mem"        // Pod's GPU Memory request for each GPU
	CountName    = "alibabacloud.com/gpu-count"      // Pod's GPU number request => Total GPU Memory == Resource * Count
	DeviceIndex  = "alibabacloud.com/gpu-index"      // Exists when the pod are assigned/predefined to a GPU device
	AssumeTime   = "alibabacloud.com/assume-time"    // To retrieve the scheduling latency
	ModelName    = "alibabacloud.com/gpu-card-model" // node annotation
)
