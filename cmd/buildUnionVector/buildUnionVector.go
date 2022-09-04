package main

import (
	"flag"
	log "github.com/sirupsen/logrus"
	pp "github.com/teamnsrg/profparse"
	"path"
	"strings"
	"sync"
)

type Task struct {
	Path string
}

var CompleteCounter int

var Accumulator []bool
var AccumLock sync.Mutex

func main() {
	var initVectorFile string
	var resultsPath string
	var outfile string

	flag.StringVar(&initVectorFile, "init-vector-file", "",
		"Path to sample text coverage file for metadata generation")
	flag.StringVar(&resultsPath, "results-path", "results",
		"Path to MIDA results for analysis")
	flag.StringVar(&outfile, "out", "file_coverage.csv",
		"Path to output file csv")
	flag.Parse()

	var err error

	if initVectorFile != "" {
		Accumulator, err = pp.ReadBVFileToBV(initVectorFile)
		if err != nil {
			log.Fatal(err)
		}
	}

	covPaths, err := pp.GetCovPathsMIDAResults(resultsPath, false)
	if err != nil {
		log.Fatal(err)
	}
	log.Infof("Retrieved paths for %d results directories", len(covPaths))
	var resultsPaths []string

	for _, c := range covPaths {
		resultsPaths = append(resultsPaths, strings.TrimSuffix(c, "coverage/coverage.bv"))
	}

	taskChan := make(chan Task, 10000)
	var wg sync.WaitGroup

	WORKERS := 28
	for i := 0; i < WORKERS; i++ {
		wg.Add(1)
		go worker(taskChan, &wg)
	}

	for _, resultsPath := range resultsPaths {
		var t Task
		t.Path = resultsPath
		taskChan <- t
	}

	close(taskChan)
	wg.Wait()
	log.Info("Workers finished")

	log.Infof("Length of accumulator to write: %d", len(Accumulator))
	err = pp.WriteFileFromBV(outfile, Accumulator)
	if err != nil {
		log.Fatal(err)
	}
}

func worker(taskChan chan Task, wg *sync.WaitGroup) {
	for task := range taskChan {

		log.Infof("Processing task: %s", task.Path)

		bv, err := pp.ReadBVFileToBV(path.Join(task.Path, "coverage", "coverage.bv"))
		if err != nil {
			log.Error(err)
			continue
		}

		AccumLock.Lock()
		if len(Accumulator) == 0 {
			Accumulator = make([]bool, len(bv))
		}

		log.Infof("Accumulator length: %d", len(Accumulator))
		log.Infof("bv length: %d", len(bv))

		var totalSoFar = 0
		Accumulator, totalSoFar, err = pp.CombineBVs([][]bool{Accumulator, bv})
		log.Infof("Accumulator length: %d", len(Accumulator))

		if err != nil {
			log.Fatal(err)
		}
		AccumLock.Unlock()

		CompleteCounter += 1
		log.Infof("Complete: %d, Total Regions So Far: %d", CompleteCounter, totalSoFar)
	}
	wg.Done()
}
