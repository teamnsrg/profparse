package main

import (
	"encoding/csv"
	"flag"
	log "github.com/sirupsen/logrus"
	pp "github.com/teamnsrg/profparse"
	"os"
	"strconv"
	"sync"
)

type Task struct {
	Path string
}

type Result struct {
	Path  string
	Same  int
	Total int
}

type BVRange struct {
	Start int //inclusive
	End   int // exclusive
}

/**
 * This analyzes region coverage across a results set.\
 * It produces two output files: one showing how many times each region was covered,
 * and one showing how many regions each site visit covered.
 */

var MetaMap map[string]map[string][]pp.CodeRegion
var Structure map[string]map[string]int
var BVIndexToCodeRegionMap map[int]pp.CodeRegion
var FilenameToBVIndices map[string]BVRange
var CompleteCounter int
var SortedFiles []string
var FileCoverage map[string][]float64

var FileCovCounts map[string]int
var FileCovCountLock sync.Mutex
var FuncCovCounts map[string]int
var FuncCovCountLock sync.Mutex

var regionCoverage []int
var regionCoverageLock sync.Mutex

var CompareMaskCovered []bool
var CompareMaskExclude []bool
var Excluded int
var Total int

func main() {
	var resultsPath string
	var outfile string

	flag.StringVar(&resultsPath, "results-path", "results",
		"Path to MIDA results for analysis")
	flag.StringVar(&outfile, "out", "output/compare_mask_similarities.csv",
		"Path to output file csv")

	flag.Parse()

	var err error
	CompleteCounter = 0

	// Build FilenameToBVIndices Map
	FilenameToBVIndices = make(map[string]BVRange)
	currentIndex := 0
	start := 0
	end := 0
	for k, v := range Structure {
		start = currentIndex
		for _, numBlocks := range v {
			currentIndex += numBlocks
		}
		end = currentIndex
		FilenameToBVIndices[k] = BVRange{
			Start: start,
			End:   end,
		}
	}

	FileCoverage = make(map[string][]float64)

	covPaths, err := pp.GetCovPathsMIDAResults(resultsPath, true)
	if err != nil {
		log.Fatal(err)
	}

	CompareMaskCovered, err = pp.ReadBVFileToBV("output/compareMaskCovered.bv")
	if err != nil {
		log.Fatal(err)
	}

	CompareMaskExclude, err = pp.ReadBVFileToBV("output/compareMaskExclude.bv")
	if err != nil {
		log.Fatal(err)
	}

	Excluded, Total = pp.CountCoveredRegions(CompareMaskCovered)

	taskChan := make(chan Task, 10000)
	resultChan := make(chan Result, 10000)
	var wg sync.WaitGroup
	var owg sync.WaitGroup

	owg.Add(1)
	go writer(resultChan, &owg, outfile)

	WORKERS := 28
	for i := 0; i < WORKERS; i++ {
		wg.Add(1)
		go worker(taskChan, resultChan, &wg)
	}

	for _, path := range covPaths {
		var t Task
		t.Path = path
		taskChan <- t
	}

	close(taskChan)
	wg.Wait()
	close(resultChan)
	owg.Wait()

	numTrials := len(covPaths)
	log.Infof("numTrials: %s", numTrials)
	log.Info("Finished")
}

func worker(taskChan chan Task, resultsChan chan Result, wg *sync.WaitGroup) {
	for task := range taskChan {
		bv, err := pp.ReadBVFileToBV(task.Path)
		if err != nil {
			log.Error(err)
			continue
		}

		same := 0
		total := 0
		for i := range bv {
			if CompareMaskExclude[i] {
				continue
			}

			total += 1
			if bv[i] == CompareMaskCovered[i] {
				same += 1
			}
		}

		CompleteCounter += 1
		log.Info(CompleteCounter)

		var r Result
		r.Path = task.Path
		r.Same = same
		r.Total = total
		resultsChan <- r
	}
	wg.Done()
}

func writer(resultChan chan Result, wg *sync.WaitGroup, outfile string) {

	f, err := os.Create(outfile)
	if err != nil {
		log.Fatal(err)
	}

	writer := csv.NewWriter(f)
	writer.Write([]string{
		"Results Path",
		"Same",
		"Total",
		"Percent",
	})

	for result := range resultChan {
		crawlPath := result.Path

		writer.Write([]string{
			crawlPath,
			strconv.Itoa(result.Same),
			strconv.Itoa(result.Total),
			strconv.FormatFloat(float64(result.Same)/float64(result.Total), 'f', 8, 64),
		})
		writer.Flush()
	}
	wg.Done()
}
