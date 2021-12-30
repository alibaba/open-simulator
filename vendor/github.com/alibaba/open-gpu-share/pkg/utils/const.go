package utils

const (
	ResourceName = "alibabacloud.com/gpu-mem"
	CountName    = "alibabacloud.com/gpu-count"
	ModelName    = "alibabacloud.com/gpu-card-model"

	EnvNVGPU              = "NVIDIA_VISIBLE_DEVICES"
	EnvResourceIndex      = "ALIBABACLOUD_COM_GPU_MEM_IDX"
	EnvResourceByPod      = "ALIBABACLOUD_COM_GPU_MEM_POD"
	EnvResourceByDev      = "ALIBABACLOUD_COM_GPU_MEM_DEV"
	EnvAssignedFlag       = "ALIBABACLOUD_COM_GPU_MEM_ASSIGNED"
	EnvResourceAssumeTime = "ALIBABACLOUD_COM_GPU_MEM_ASSUME_TIME"
)
