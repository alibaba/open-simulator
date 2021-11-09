package utils

const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorCyan   = "\033[36m"

	// open-local storage class names
	OpenLocalSCNameLVM           = "open-local-lvm"
	OpenLocalSCNameDeviceHDD     = "open-local-device-hdd"
	OpenLocalSCNameDeviceSSD     = "open-local-device-ssd"
	OpenLocalSCNameMountPointHDD = "open-local-mountpoint-hdd"
	OpenLocalSCNameMountPointSSD = "open-local-mountpoint-ssd"

	// yoda storage class names
	YodaSCNameLVM           = "yoda-lvm-default"
	YodaSCNameDeviceHDD     = "yoda-device-hdd"
	YodaSCNameDeviceSSD     = "yoda-device-ssd"
	YodaSCNameMountPointHDD = "yoda-mountpoint-hdd"
	YodaSCNameMountPointSSD = "yoda-mountpoint-ssd"
)
