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

/**
 * This builds a csv containing data on files that are covered by 1%, 10%, 50%, 99%, etc. of sites
 */

type Task struct {
	Path string
}

type Result struct {
	Path      string
	FinalTree map[string]int
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

	MetaMap, _, err = pp.ReadCovMetadata(covFile)
	if err != nil {
		log.Fatal(err)
	}

	FileCovCounts = make(map[string]int)

	sampleCovMap, _, err := pp.ReadFileToCovMap(covFile)
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

	covPaths, err := pp.GetCovPathsMIDAResults(resultsPath, false)
	if err != nil {
		log.Fatal(err)
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
		go fileWorker(taskChan, &wg)
	}

	for _, path := range covPaths {
		var t Task
		t.Path = path
		taskChan <- t
	}

	close(taskChan)
	wg.Wait()
	log.Info("Workers finished, starting")

	// At this point, we should have a complete RegionCovCounts vector, and we can build a tree

	numTrials := len(covPaths)
	log.Infof("numTrials: %s", numTrials)

	outfileName := outfile

	f, err := os.Create(outfileName)
	if err != nil {
		log.Fatal(err)
	}
	writer := csv.NewWriter(f)
	writer.Write([]string{
		"Filename",
		"Number of Regions",
		"1% Regions Covered",
		"10% Regions Covered",
		"25% Regions Covered",
		"50% Regions Covered",
		"75% Regions Covered",
		"99% Regions Covered",
	})

	for fname := range FileCoverage {
		p1 := 0
		p10 := 0
		p25 := 0
		p50 := 0
		p75 := 0
		p99 := 0
		for _, val := range FileCoverage[fname] {
			if val > 0.01 {
				p1 += 1
				if val > 0.1 {
					p10 += 1
					if val > 0.25 {
						p25 += 1
						if val > 0.50 {
							p50 += 1
							if val > 0.75 {
								p75 += 1
								if val > 0.99 {
									p99 += 1
								}
							}
						}
					}
				}
			}
		}

		writer.Write([]string{fname,
			fname,
			strconv.Itoa(FilenameToBVIndices[fname].End - FilenameToBVIndices[fname].Start),
			strconv.Itoa(p1),
			strconv.Itoa(p10),
			strconv.Itoa(p25),
			strconv.Itoa(p50),
			strconv.Itoa(p75),
			strconv.Itoa(p99),
		})
		writer.Flush()
	}

	f.Close()

}

func fileWorker(taskChan chan Task, wg *sync.WaitGroup) {
	for task := range taskChan {
		bv, err := pp.ReadBVFileToBV(task.Path)
		if err != nil {
			log.Error(err)
			continue
		}

		for _, fname := range SortedFiles {
			indices := FilenameToBVIndices[fname]
			fileCov := 0
			for i := indices.Start; i < indices.End; i++ {
				if bv[i] {
					fileCov += 1
				}
			}

			FileCovCountLock.Lock()
			if _, ok := FileCoverage[fname]; !ok {
				FileCoverage[fname] = make([]float64, 0)
			}
			FileCoverage[fname] = append(FileCoverage[fname], float64(fileCov)/float64(indices.End-indices.Start))

			FileCovCountLock.Unlock()
		}

		CompleteCounter += 1
		log.Info(CompleteCounter)
	}
	wg.Done()
}

func ConvertFileCoverageToTree(fc map[string]int) map[string]int {
	tree := make(map[string]int)
	for k, v := range fc {
		parts := strings.Split(k, "/")
		if parts[0] == ".." && parts[1] == ".." {
			parts = parts[2:]
		} else if parts[0] == "gen" {
			parts = parts[1:]
		}

		for i := 0; i < len(parts); i++ {
			seg := strings.Join(parts[:i+1], "/")
			if _, ok := tree[seg]; !ok {
				tree[seg] = 0
			}
			tree[seg] += v
		}
	}

	return tree
}
