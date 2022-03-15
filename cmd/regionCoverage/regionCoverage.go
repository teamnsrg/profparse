package main

import (
	"encoding/csv"
	"flag"
	log "github.com/sirupsen/logrus"
	pp "github.com/teamnsrg/profparse"
	"os"
	"sort"
	"strconv"
	"sync"
)

type Task struct {
	Path string
}

type Result struct {
	Path    string
	Covered int
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
var DenominatorFileCoverageMap map[string]int
var DenominatorTree map[string]int
var FileCoverage map[string][]float64

var FileCovCounts map[string]int
var FileCovCountLock sync.Mutex
var regionCoverage []int
var regionCoverageLock sync.Mutex

func main() {
	var covFile string
	var resultsPath string
	var outfile string
	var crawlRegionCoverageOutfile string
	var onePerSite bool

	flag.StringVar(&covFile, "coverage-file", "coverage.txt",
		"Path to sample text coverage file for metadata generation")
	flag.StringVar(&resultsPath, "results-path", "results",
		"Path to MIDA results for analysis")
	flag.StringVar(&outfile, "region-out", "region_coverage.csv",
		"Path to output file csv")
	flag.StringVar(&crawlRegionCoverageOutfile, "crawl-out", "crawl_region_coverage.csv",
		"Path to output file csv")
	flag.BoolVar(&onePerSite, "one-per-site", false,
		"If true, only one crawl per site will be counted")

	flag.Parse()

	var err error
	CompleteCounter = 0
	// log.SetReportCaller(true)
	log.Infof("Begin creating metadata structures by reading %s...", covFile)

	MetaMap, err = pp.ReadCovMetadata(covFile)
	if err != nil {
		log.Fatal(err)
	}

	FileCovCounts = make(map[string]int)

	sampleCovMap, err := pp.ReadFileToCovMap(covFile)
	if err != nil {
		log.Fatal(err)
	}

	files := 0
	functions := 0
	regions := 0

	Structure = pp.ConvertCovMapToStructure(sampleCovMap)
	for _, v1 := range Structure {
		files += 1
		for _, v2 := range v1 {
			functions += 1
			regions += v2
		}
	}
	BVIndexToCodeRegionMap = pp.GenerateBVIndexToCodeRegionMap(Structure, MetaMap)

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

	log.Infof("Finished parsing metadata")
	log.Infof("  - Total Files: %d", files)
	log.Infof("  - Total Functions: %d", functions)
	log.Infof("  - Total Code Regions: %d\n", regions)

	covPaths, err := pp.GetCovPathsMIDAResults(resultsPath, onePerSite)
	if err != nil {
		log.Fatal(err)
	}

	regionCoverage = make([]int, regions)

	sort.Strings(covPaths)
	SortedFiles = make([]string, 0)
	for k := range Structure {
		SortedFiles = append(SortedFiles, k)
	}
	sort.Strings(SortedFiles)

	taskChan := make(chan Task, 10000)
	resultChan := make(chan Result, 10000)
	var wg sync.WaitGroup
	var owg sync.WaitGroup

	owg.Add(1)
	go writer(resultChan, &owg, crawlRegionCoverageOutfile)

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

	outfileName := outfile

	f, err := os.Create(outfileName)
	if err != nil {
		log.Fatal(err)
	}
	writer := csv.NewWriter(f)
	writer.Write([]string{"File", "Function", "Region Number", "Times Covered", "Percent Times Covered"})

	for i, val := range regionCoverage {
		codeRegion := BVIndexToCodeRegionMap[i]
		writer.Write([]string{
			codeRegion.FileName,
			codeRegion.FuncName,
			strconv.Itoa(i),
			strconv.Itoa(val),
			strconv.FormatFloat(float64(val)/float64(numTrials), 'f', 4, 64),
		})

	}
}

func worker(taskChan chan Task, resultsChan chan Result, wg *sync.WaitGroup) {
	for task := range taskChan {
		bv, err := pp.ReadBVFileToBV(task.Path)
		if err != nil {
			log.Error(err)
			continue
		}

		regionsCovered := 0

		// Count number of regions covered, and ensure that any covered regions are marked as such
		for i, bit := range bv {
			if bit {
				regionsCovered++
				regionCoverageLock.Lock()
				regionCoverage[i] += 1
				regionCoverageLock.Unlock()
			}
		}

		CompleteCounter += 1
		log.Info(CompleteCounter)

		var r Result
		r.Path = task.Path
		r.Covered = regionsCovered
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
		"Regions Covered",
	})

	for result := range resultChan {
		crawlPath := result.Path
		regions := result.Covered

		writer.Write([]string{crawlPath, strconv.Itoa(regions)})
		writer.Flush()
	}
	wg.Done()
}
