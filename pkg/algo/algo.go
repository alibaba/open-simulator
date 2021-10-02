package algo

type SchedulingQueueSort interface {
	Len() int
	Swap(i, j int)
	Less(i, j int) bool
}
