package processer

import "github.com/WangYihang/Proxy-Verifier/internal/model"

func ScheduleTask(task *model.Task, taskQueue chan *model.Task, state *model.State) {
	taskQueue <- task
	state.TaskScheduled()
}
