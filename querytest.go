package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

type queryTestPassType int

const (
	testPassIgnore queryTestPassType = iota
	testPassNoError
	testPassRows
	testPassOneRowOnly
)

const testMaxRows = 10

func (tp queryTestPassType) String() string {
	return [...]string{"ignore", "noerror", "rows", "onerowonly"}[tp]
}

func (tp *queryTestPassType) parse(str string) error {
	switch str {
	case testPassIgnore.String():
		*tp = testPassIgnore
	case testPassNoError.String():
		*tp = testPassNoError
	case testPassRows.String():
		*tp = testPassRows
	case testPassOneRowOnly.String():
		*tp = testPassOneRowOnly
	default:
		return errors.New("Invalid testpass string: " + str)
	}
	return nil
}

type queryTestReport struct {
	startTime  time.Time
	endTime    time.Time
	testResult []byte
}

// main autotest func
func (srvc *pgmusql) queryTestRun(workers uint, ignorerrors bool) error {
	// create tasks queue
	task := make(chan *query, len(srvc.queries))
	for _, value := range srvc.queries {
		if value.err != nil {
			if !ignorerrors {
				return value.err
			}
			continue
		}
		task <- value
	}
	close(task)
	taskCount := len(task)

	// spawn test workers
	errch := make(chan error)
	defer close(errch)

	ctrl := make(chan bool)
	defer close(ctrl)

	ctx, ctxCancelFnc := context.WithCancel(srvc.mainContext)
	defer ctxCancelFnc()

	for i := uint(0); i < workers; i++ {
		go srvc.queryTestWorker(ctx, task, errch, ctrl, ignorerrors, i+1)
	}

	// handel result
	for i := 0; i < taskCount; i++ {
		ctrl <- true
		if err := <-errch; err != nil {
			return err
		}
	}

	return nil
}

func (srvc *pgmusql) queryTestWorker(ctx context.Context, task <-chan *query, result chan<- error, ctrl <-chan bool, ignorerrors bool, id uint) {
	log.Printf("Run %d test worker\n", id)
	defer log.Printf("Test worker %d stop\n", id)

	for t := range task {
		log.Printf("Test worker (%d): in que %d run query %s\n", id, len(task), t.name)
		err := srvc.queryTestScenario(ctx, t, ignorerrors)
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ctrl:
			if !ok {
				return
			}
			result <- err
		}
	}
}

// autotest scenario
func (srvc *pgmusql) queryTestScenario(ctx context.Context, query *query, ignorerrors bool) error {
	if query.testpass == testPassIgnore {
		return nil
	}

	// error handling
	handleErr := func(perr error) error {
		err := errors.New("Autotest error: " + perr.Error())
		if ignorerrors {
			query.err = err
			return nil
		}
		return err
	}

	testStartTime := time.Now()

	params := query.testparams.toURLValues()
	result, total, err := srvc.runQuery(ctx, query.name, params, testMaxRows)

	if err != nil {
		return handleErr(err)
	}

	switch query.testpass {
	case testPassNoError: // nothing we allready check all errors
	case testPassRows:
		if total <= 0 {
			return handleErr(fmt.Errorf("Failed test scenario \"rows\". Result rows: %d", total))
		}
	case testPassOneRowOnly:
		if total != 1 {
			return handleErr(fmt.Errorf("Failed test scenario \"onerowonly\". Result rows: %d", total))
		}
	}

	query.testreport = &queryTestReport{
		startTime:  testStartTime,
		endTime:    time.Now(),
		testResult: result,
	}

	return nil
}
