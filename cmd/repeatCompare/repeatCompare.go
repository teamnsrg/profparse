package main

import (
	"encoding/csv"
	"flag"
	log "github.com/sirupsen/logrus"
	pp "github.com/teamnsrg/profparse"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type Task struct {
	Path string
}

type CoverageComparison struct {
	CovPathOne      string
	CovPathTwo      string
	DomainOne       string
	DomainTwo       string
	RegionsCompared int
	RegionsDiff     int
}

type Result struct {
	Domain      string
	Comparisons []CoverageComparison
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

var ExcludeBV []bool

func main() {
	var covFile string
	var resultsPath string
	var excludeBVFile string
	var outfile string

	flag.StringVar(&covFile, "coverage-file", "coverage.txt",
		"Path to sample text coverage file for metadata generation")
	flag.StringVar(&resultsPath, "results-path", "results",
		"Path to MIDA results for analysis")
	flag.StringVar(&excludeBVFile, "exclude-bv", "",
		"Path to exclude bit vector")
	flag.StringVar(&outfile, "outfile", "output/same_compare.csv",
		"Path to output file")

	flag.Parse()

	var err error
	CompleteCounter = 0
	// log.SetReportCaller(true)
	log.Infof("Begin creating metadata structures by reading %s...", covFile)

	MetaMap, _, err = pp.ReadCovMetadata(covFile)
	if err != nil {
		log.Fatal(err)
	}

	FileCovCounts = make(map[string]int)
	FuncCovCounts = make(map[string]int)

	sampleCovMap, _, err := pp.ReadFileToCovMap(covFile)
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

	if excludeBVFile == "" {
		ExcludeBV = make([]bool, regions)
	} else {
		ExcludeBV, err = pp.ReadBVFileToBV(excludeBVFile)
		if err != nil {
			log.Fatal(err)
		}
	}

	FileCoverage = make(map[string][]float64)

	log.Infof("Finished parsing metadata")
	log.Infof("  - Total Files: %d", files)
	log.Infof("  - Total Functions: %d", functions)
	log.Infof("  - Total Code Regions: %d\n", regions)

	sitePaths, err := pp.GetSitePathsMidaResults(resultsPath)

	regionCoverage = make([]int, regions)

	sort.Strings(sitePaths)
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
	go writer(resultChan, &owg, outfile)

	WORKERS := 28
	for i := 0; i < WORKERS; i++ {
		wg.Add(1)
		go worker(taskChan, resultChan, &wg)
	}

	log.Infof("Loading %d site paths...", len(sitePaths))

	for _, path := range sitePaths {
		var t Task
		t.Path = path
		taskChan <- t
	}

	close(taskChan)
	wg.Wait()
	close(resultChan)
	owg.Wait()

	numSites := len(sitePaths)
	log.Infof("numSites: %s", numSites)

	log.Info("Finished")
}

func worker(taskChan chan Task, resultsChan chan Result, wg *sync.WaitGroup) {
	for task := range taskChan {
		covPaths, err := pp.GetCovPathsSite(task.Path)
		if err != nil {
			log.Error(err)
			continue
		}

		var r Result
		parts := strings.Split(task.Path, "/")
		r.Domain = parts[len(parts)-1]
		r.Comparisons = make([]CoverageComparison, 0)

		bvMap := make(map[string][]bool)
		for _, covPath := range covPaths {
			bv, err := pp.ReadBVFileToBV(covPath)
			if err != nil {
				log.Error(err)
				continue
			}

			bvMap[covPath] = bv
		}

		for covPathOne, bvOne := range bvMap {
			for covPathTwo, bvTwo := range bvMap {
				if covPathOne == covPathTwo {
					continue
				}

				diff, total, err := pp.DiffBVsWithExclude(bvOne, bvTwo, ExcludeBV)
				if err != nil {
					log.Error(err)
					continue
				}

				var cc CoverageComparison
				cc.DomainOne = r.Domain
				cc.DomainTwo = r.Domain
				cc.CovPathOne = covPathOne
				cc.CovPathTwo = covPathTwo
				cc.RegionsDiff = diff
				cc.RegionsCompared = total

				r.Comparisons = append(r.Comparisons, cc)
			}
		}

		resultsChan <- r
	}
	wg.Done()
}

func writer(resultChan chan Result, wg *sync.WaitGroup, outfile string) {

	log.Infof("Writing output to: %s", outfile)
	f, err := os.Create(outfile)
	if err != nil {
		log.Fatal(err)
	}

	completed := 0

	writer := csv.NewWriter(f)
	writer.Write([]string{
		"Same or Different",
		"Domain One",
		"Domain Two",
		"Path One",
		"Path Two",
		"Different Regions",
		"Regions Compared",
		"Percent Difference",
	})

	for result := range resultChan {
		for _, comp := range result.Comparisons {
			writer.Write([]string{
				"Same-Gremlins",
				comp.DomainOne,
				comp.DomainTwo,
				comp.CovPathOne,
				comp.CovPathTwo,
				strconv.Itoa(comp.RegionsDiff),
				strconv.Itoa(comp.RegionsCompared),
				strconv.FormatFloat(float64(comp.RegionsDiff)/float64(comp.RegionsCompared), 'f', 8, 64),
			})
			writer.Flush()
		}

		completed += 1
		log.Infof("Sites completed: %d", completed)
	}

	f.Close()

	wg.Done()
}
