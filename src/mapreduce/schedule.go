package mapreduce

import (
	"fmt"
)

// schedule starts and waits for all tasks in the given phase (Map or Reduce).
func (mr *Master) schedule(phase jobPhase) {
	var ntasks int
	var nios int // number of inputs (for reduce) or outputs (for map)
	switch phase {
	case mapPhase:
		ntasks = len(mr.files)
		nios = mr.nReduce
	case reducePhase:
		ntasks = mr.nReduce
		nios = len(mr.files)
	}

	fmt.Printf("Schedule: %v %v tasks (%d I/Os)\n", ntasks, phase, nios)
	// consider the following strategy:
	// create a bool array to storage the completion state of each task
	// create done channel;
	// create goroutines for each jobs assigned;when rpc returned,send corresponding
	//        worker string into channel
	// on receiving channel messeage,attempting to assign a new task for that worker
	//        using goroutine
	completionChan := make(chan string) // corresponding worker string
	jobState := make([]int, ntasks)     // storage current jobs done state

	// init jobState array
	for jsIt := 0; jsIt < ntasks; jsIt++ {
		jobState[jsIt] = -1
	}
	// init workers completion array
	if len(mr.workers) != 0 {
		for it := 0; it < len(mr.workers); it++ {
			go func(wk string) {
				completionChan <- wk
			}(mr.workers[it])
		}
	}

	// waiting for registration/completion using select
	for {
		fmt.Printf("Entered for loop.\n")
		FLAG_EVERYTHING_DONE := true
		var tmpArgs DoTaskArgs
		tmpArgs.NumOtherPhase = nios
		tmpArgs.JobName = mr.jobName
		tmpArgs.Phase = phase
		select {
		case onComplete := <-completionChan:
			// select a job
			fmt.Printf("Someone completed its task!\n")
			jIt := 0
			for ; jIt < ntasks; jIt++ {
				if jobState[jIt] == -1 {
					FLAG_EVERYTHING_DONE = false
					break
				}
			}
			if FLAG_EVERYTHING_DONE {
				break
			}
			fmt.Printf("Start Assigning new jobs...\n")
			tmpArgs.TaskNumber = jIt
			tmpArgs.File = mr.files[jIt]
			// set jobstate to working
			jobState[jIt] = 0
			go func() {
				wkErr := call(onComplete, "Worker.DoTask", tmpArgs, nil)
				jobState[jIt] = 1
				if !wkErr {
					fmt.Printf("Debug: a worker died.\n")
					jobState[jIt] = -1
					return
				}
				completionChan <- onComplete
			}()

		case onRegister := <-mr.registerChannel:
			fmt.Printf("Someone has come online!\n")
			jIt := 0
			for ; jIt < ntasks; jIt++ {
				if jobState[jIt] == -1 {
					FLAG_EVERYTHING_DONE = false
					break
				}
			}
			if FLAG_EVERYTHING_DONE {
				break
			}
			fmt.Printf("Start Assigning new jobs...")
			tmpArgs.TaskNumber = jIt
			tmpArgs.File = mr.files[jIt]
			jobState[jIt] = 0
			go func() {
				wkErr := call(onRegister, "Worker.DoTask", tmpArgs, nil)
				jobState[jIt] = 1
				if !wkErr {
					fmt.Printf("Debug: a worker died.")
					jobState[jIt] = -1
					return
				}
				completionChan <- onRegister
			}()
		}
		fmt.Printf("Start checking overall completion state...\n")
		FLAG_ALL := false
		if FLAG_EVERYTHING_DONE {
			FLAG_ALL = true
			// that indicates there's no job in queue
			// check is everything OK
			for it := 0; it < ntasks; it++ {
				if jobState[it] != 1 {
					fmt.Printf("Job %d haven't completed yet.\n", it)
					FLAG_ALL = false
					break
				}
			}
		}
		if FLAG_ALL {
			fmt.Printf("Schedule: %v phase done\n", phase)
			return
		}
	}

	// -----------------------------------------------------------------------------
	// All ntasks tasks have to be scheduled on workers, and only once all of
	// them have been completed successfully should the function return.
	// Remember that workers may fail, and that any given worker may finish
	// multiple tasks.
	//
	// TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO TODO
	//

}
