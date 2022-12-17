package main

import (
	"log"
	"os"
	"runtime/trace"
	"syscall"
	"time"
)

func main() {
	// take a runtime trace to evaluate the behavior
	file, err := os.Create("main.trace")
	if err != nil {
		panic(err)
	} else if err := trace.Start(file); err != nil {
		panic(err)
	}
	defer trace.Stop()

	for i := 0; i < 2; i++ {
		go foregroundWork(time.Second)
	}
	go backgroundWork()
	time.Sleep(6 * time.Second)
}

// foregroundWork runs a loop that does CPU heavy work for one period followed
// by one period of sleep.
func foregroundWork(period time.Duration) {
	timer := time.After(period)
	for {
		select {
		case <-timer:
			time.Sleep(period)
			timer = time.After(period)
		default:
			_ = "burn cpu cycles"
		}
	}
}

func backgroundWork() {
	period := 100 * time.Millisecond
	u := NewCPUUtilization(period)
	threshold := float64(1.5)

	for {
		cores := <-u.C
		if cores > threshold {
			continue
		}
		log.Printf("starting background work: cores=%.2f < %.2f", cores, threshold)

	workLoop:
		for {
			select {
			case cores := <-u.C:
				if cores > threshold {
					log.Printf("stopping background work: cores: %.2f > %.2f", cores, threshold)
					break workLoop
				}
			default:
				_ = "do a small amount of CPU work here"
			}
		}
	}
}

func NewCPUUtilization(period time.Duration) *CPUUtilization {
	c := &CPUUtilization{
		C:    make(chan float64, 1),
		stop: make(chan struct{}),
	}
	go c.measure(period)
	return c
}

type CPUUtilization struct {
	C    chan float64
	stop chan struct{}
}

func (c *CPUUtilization) measure(period time.Duration) {
	var before syscall.Rusage
	var after syscall.Rusage
	t := time.NewTicker(period)
	defer t.Stop()
	for {
		start := time.Now()
		beforeErr := syscall.Getrusage(syscall.RUSAGE_SELF, &before)
		select {
		case <-t.C:
		case <-c.stop:
			return
		}
		afterErr := syscall.Getrusage(syscall.RUSAGE_SELF, &after)

		var cores float64
		if beforeErr != nil || afterErr != nil {
			cores = -1 // should be impossible according to getrusage(2) docs, but let's handle it
		} else {
			cpuNano := after.Utime.Nano() + after.Stime.Nano() - before.Utime.Nano() - before.Stime.Nano()
			cores = float64(cpuNano) / float64(time.Since(start))
		}

		select {
		case c.C <- cores:
		case <-c.stop:
			return
		default:
			continue
		}
	}
}

// Stop stops the cpu utilization measurement.
func (c *CPUUtilization) Stop() {
	select {
	case <-c.stop:
		return
	default:
		close(c.stop)
	}
}
