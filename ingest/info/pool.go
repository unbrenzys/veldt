package info

import (
	"sync"

	"github.com/unchartedsoftware/prism/ingest/es"
	"github.com/unchartedsoftware/prism/ingest/progress"
)

// Worker represents a designated worker function to batch in a pool.
type Worker func(IngestFile, *es.Equalizer) error

// Pool represents a single goroutine pool for batching workers.
type Pool struct {
	FileChan  chan IngestFile
	ErrChan   chan error
	WaitGroup *sync.WaitGroup
	Size      int
}

// NewPool returns a new pool object with the given worker size
func NewPool(size int) *Pool {
	return &Pool{
		FileChan:  make(chan IngestFile),
		ErrChan:   make(chan error),
		WaitGroup: new(sync.WaitGroup),
		Size:      size,
	}
}

func sendError(errChan chan error, err error) {
	errChan <- err
}

func workerWrapper(p *Pool, eq *es.Equalizer, worker Worker, ingestInfo *IngestInfo) {
	// Decrease internal counter for wait-group as soon as goroutine finishes
	defer p.WaitGroup.Done()
	for file := range p.FileChan {
		// do work
		err := worker(file, eq)
		// if error, broadcast to pool
		if err != nil {
			// broadcast error message and continue grabbing from pool
			// we don't just return because the pool will be blocked on a pending
			// file and we need another worker to grab it
			go sendError(p.ErrChan, err)
			continue
		}
		// Update and print current progress
		progress.UpdateProgress(file.Size)
	}
}

// Execute launches a batch of ingest workers with the provided ingest information.
func (p *Pool) Execute(worker Worker, ingestInfo *IngestInfo) error {
	// create equalizer of same size
	eq := es.NewEqualizer(p.Size)
	eq.Listen()
	defer eq.Close()
	// for each worker in pool
	for i := 0; i < p.Size; i++ {
		// increase wait group size
		p.WaitGroup.Add(1)
		// dispatch the workers, they will wait until the input channel is closed
		go workerWrapper(p, eq, worker, ingestInfo)
	}
	// start progress tracking
	progress.StartProgress(ingestInfo.NumTotalBytes)
	// process all files by spreading them to free workers, this blocks until
	// a worker is available, or exits if there is an error
	for _, file := range ingestInfo.Files {
		select {
		case err := <-p.ErrChan:
			// if error has occured, exit with error
			close(p.FileChan)
			close(p.ErrChan)
			return err
		default:
			// if not, continue passing files to workers
			p.FileChan <- file
		}
	}
	// close channels to allow the worker goroutines to end execution
	close(p.FileChan)
	close(p.ErrChan)
	// wait for all workers to finish (otherwise they die as main routine dies)
	p.WaitGroup.Wait()
	// end progress tracking, and print summary
	progress.EndProgress()
	progress.PrintTotalDuration()
	return nil
}
