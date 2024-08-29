package goramcache

import (
	"runtime"
	"sync"
	"time"
)

type Janitor struct {
	nextTimeExpire time.Time
	timer          *time.Timer
	interval       time.Duration
	stop           chan bool
	sync.RWMutex
}

func NewJanitor(interval time.Duration) *Janitor {
	return &Janitor{
		interval: interval,
		stop:     make(chan bool),
		timer:    time.NewTimer(0),
	}
}

func (j *Janitor) ResetDefault() {
	j.Lock()
	j.timer.Reset(j.interval)
	j.Unlock()
}

func (j *Janitor) SetNextCheckExpire(moreTime time.Duration) {
	j.Lock()
	j.timer.Reset(moreTime)
	j.Unlock()
}

func (j *Janitor) SetInterval(interval time.Duration) {
	j.Lock()
	j.interval = interval
	j.Unlock()
}

func (j *Janitor) GetInterval() time.Duration {
	j.Lock()
	defer j.Unlock()
	return j.interval
}

func (j *Janitor) Start(obj any, f func() (nextCheckAt time.Time, needUpdateForNext bool)) {
	if j.interval <= 0 {
		return
	}
	go func() {
		var nextTime time.Time
		var ok bool
		j.timer = time.NewTimer(0) //do the test right the first time
		j.nextTimeExpire = time.Now().Add(j.interval)
		for {
			select {
			case <-j.timer.C:
				// fmt.Println("Checking Exprite ", time.Now())
				nextTime, ok = f()
				j.Lock()
				if ok && j.nextTimeExpire.After(nextTime) {
					j.timer.Reset(time.Until(nextTime))
					j.nextTimeExpire = nextTime
				} else {
					j.timer.Reset(j.interval)
					j.nextTimeExpire = time.Now().Add(j.interval)
				}
				j.Unlock()
			case <-j.stop: // send by runtime.SetFinalizer
				j.timer.Stop()
				return
			}
		}
	}()
	runtime.SetFinalizer(obj, func(obj any) {
		// fmt.Println("Exit janitor")
		j.stop <- true
	})
}
