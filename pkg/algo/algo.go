package algo

// SchedulingQueueSort is interface for sorting pods
type SchedulingQueueSort interface {
	Len() int
	Swap(i, j int)
	Less(i, j int) bool
}
