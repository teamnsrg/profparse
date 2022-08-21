package main

import (
	"flag"
	log "github.com/sirupsen/logrus"
	pp "github.com/teamnsrg/profparse"
	"sort"
	"strings"
	"sync"
)

type Task struct {
	Path string
}

type Result struct {
	Path string
}

type BVRange struct {
	Start int //inclusive
	End   int // exclusive
}

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

func main() {
	var covFile string
	var resultsPath string
	var outfile string

	flag.StringVar(&covFile, "coverage-file", "coverage.txt",
		"Path to sample text coverage file for metadata generation")
	flag.StringVar(&resultsPath, "results-path", "results",
		"Path to MIDA results for analysis")
	flag.StringVar(&outfile, "out", "file_coverage.csv",
		"Path to output file csv")

	flag.Parse()

	var err error
	CompleteCounter = 0
	log.SetReportCaller(true)
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

	// sampleBV := pp.ConvertCovMapToBools(sampleCovMap)

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

	covPaths, err := pp.GetCovPathsMIDAResults(resultsPath, true)
	if err != nil {
		log.Fatal(err)
	}
	log.Infof("Retrieved paths for %d results directories", len(covPaths))
	var resultsPaths []string

	for _, c := range covPaths {
		resultsPaths = append(resultsPaths, strings.TrimSuffix(c, "coverage/coverage.bv"))
	}

	sort.Strings(covPaths)
	SortedFiles = make([]string, 0)
	for k := range Structure {
		SortedFiles = append(SortedFiles, k)
	}
	sort.Strings(SortedFiles)

	taskChan := make(chan Task, 10000)
	var wg sync.WaitGroup

	WORKERS := 28
	for i := 0; i < WORKERS; i++ {
		wg.Add(1)
		go worker(taskChan, &wg)
	}

	for _, path := range resultsPaths {
		var t Task
		t.Path = path
		taskChan <- t
	}

	close(taskChan)
	wg.Wait()
	log.Info("Workers finished, starting")

	numTrials := len(covPaths)
	log.Infof("numTrials: %d", numTrials)
}

func worker(taskChan chan Task, wg *sync.WaitGroup) {
	for task := range taskChan {

		log.Infof("Processing task: %s", task.Path)

		//bv, err := pp.ReadBVFileToBV(task.Path)
		//if err != nil {
		//	log.Error(err)
		//	continue
		//}

		//for _, fname := range SortedFiles {
		//	indices := FilenameToBVIndices[fname]
		//	fileCov := 0
		//	for i := indices.Start; i < indices.End; i++ {
		//		if bv[i] {
		//			fileCov += 1
		//		}
		//	}
		//
		//	FileCovCountLock.Lock()
		//	if _, ok := FileCoverage[fname]; !ok {
		//		FileCoverage[fname] = make([]float64, 0)
		//	}
		//	FileCoverage[fname] = append(FileCoverage[fname], float64(fileCov)/float64(indices.End-indices.Start))
		//
		//	FileCovCountLock.Unlock()
		//}

		CompleteCounter += 1
		log.Info(CompleteCounter)
	}
	wg.Done()
}
