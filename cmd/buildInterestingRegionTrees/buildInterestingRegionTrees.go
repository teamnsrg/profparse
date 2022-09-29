package main

import (
	"encoding/csv"
	"flag"
	log "github.com/sirupsen/logrus"
	pp "github.com/teamnsrg/profparse"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
)

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
var SortedFuncs map[string][]string
var DenominatorFileCoverageMap map[string]int
var DenominatorTree map[string]int
var TotalBVs int
var RegionCovCounts []int
var RegionCountLock sync.Mutex

func main() {
	var covFile string
	var resultsPath string
	var outfiledir string
	var excludeBVPath string

	flag.StringVar(&covFile, "coverage-file", "",
		"Path to sample text coverage file for metadata generation")
	flag.StringVar(&resultsPath, "results-path", "",
		"Path to MIDA results for analysis")
	flag.StringVar(&excludeBVPath, "exclude-bv", "",
		"Path to BV file to use for region exclusion")
	flag.StringVar(&outfiledir, "out", "file_coverage",
		"Path to output file directory")

	flag.Parse()

	var err error
	CompleteCounter = 0
	log.SetReportCaller(true)
	log.Infof("Begin creating metadata structures by reading %s...", covFile)

	MetaMap, _, err = pp.ReadCovMetadata(covFile)
	if err != nil {
		log.Fatal(err)
	}

	sampleCovMap, _, err := pp.ReadFileToCovMap(covFile)
	if err != nil {
		log.Fatal(err)
	}

	files := 0
	functions := 0
	regions := 0

	sampleBV := pp.ConvertCovMapToBools(sampleCovMap)
	RegionCovCounts = make([]int, len(sampleBV))

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

	fileNames := make([]string, 0)
	for k := range Structure {
		fileNames = append(fileNames, k)
	}
	sort.Strings(fileNames)

	for _, k := range fileNames {
		start = currentIndex
		funcNames := make([]string, 0)
		for funcName := range Structure[k] {
			funcNames = append(funcNames, funcName)
		}
		sort.Strings(funcNames)

		for _, funcName := range funcNames {
			currentIndex += Structure[k][funcName]
		}
		end = currentIndex
		FilenameToBVIndices[k] = BVRange{
			Start: start,
			End:   end,
		}
	}

	excludeBV := make([]bool, len(sampleBV))
	if excludeBVPath != "" {
		excludeBV, err = pp.ReadBVFileToBV(excludeBVPath)
		if err != nil {
			log.Fatal(err)
		}
	}

	excludedRegions, _ := pp.CountCoveredRegions(excludeBV)
	log.Infof("Excluding %d regions from '%s'", excludedRegions, excludeBVPath)

	SortedFiles = make([]string, 0)
	SortedFuncs = make(map[string][]string)
	DenominatorFileCoverageMap = make(map[string]int)
	for k := range Structure {
		SortedFiles = append(SortedFiles, k)
		funcs := make([]string, 0)
		DenominatorFileCoverageMap[k] = 0
		fileRange := FilenameToBVIndices[k]
		for i := fileRange.Start; i < fileRange.End; i++ {
			if !excludeBV[i] {
				DenominatorFileCoverageMap[k] += 1
			}
		}
		for k2 := range Structure[k] {
			funcs = append(funcs, k2)
		}
		sort.Strings(funcs)
		SortedFuncs[k] = funcs
	}
	sort.Strings(SortedFiles)
	DenominatorTree = ConvertFileCoverageToTree(DenominatorFileCoverageMap)

	log.Infof("Finished parsing metadata")
	log.Infof("  - Total Files: %d", files)
	log.Infof("  - Total Functions: %d", functions)
	log.Infof("  - Total Code Regions: %d\n", regions)

	//covPaths, err := pp.GetCovPathsMIDAResults(resultsPath, true)
	//if err != nil {
	//	log.Fatal(err)
	//}

	covPaths := []string{
		//"/home/pmurley/go/src/github.com/teamnsrg/profparse/output/100k_median.bv",
		"/data3/nsrg/mida_results/VVNN-100k/cointelegraph.com/15ffc8c2-c2bb-408b-b799-992f4f800f67/coverage/coverage.bv",
		//"/data3/nsrg/mida_results/VVNN-100k/google.com/f96fa0de-43ef-4e37-b4b2-add1389c299a/coverage/coverage.bv",
	}

	sort.Strings(covPaths)

	taskChan := make(chan Task, 10000)
	var wg sync.WaitGroup

	WORKERS := 28
	for i := 0; i < WORKERS; i++ {
		wg.Add(1)
		go regionCounterWorker(taskChan, &wg)
	}

	TotalBVs = len(covPaths)
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
	interestingRegionsPerFile := make(map[string]int)
	for _, fileName := range SortedFiles {
		bvRange := FilenameToBVIndices[fileName]
		interestingRegionsInFile := 0
		for i := bvRange.Start; i < bvRange.End; i++ {
			if excludeBV[i] {
				continue
			}
			blockFrequency := float64(RegionCovCounts[i]) / float64(numTrials)
			if blockFrequency >= 0.999 {
				interestingRegionsInFile += 1
			}
		}

		interestingRegionsPerFile[fileName] = interestingRegionsInFile
	}

	finalTree := ConvertFileCoverageToTree(interestingRegionsPerFile)

	outfileName := "output/coinTelegraphCoverageTreeWithExclude.csv"

	f, err := os.Create(outfileName)
	if err != nil {
		log.Fatal(err)
	}

	var keys []string
	for k := range finalTree {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	writer := csv.NewWriter(f)
	writer.Write([]string{"names", "parents", "covered", "total", "percentcovered"})

	for _, name := range keys {
		parts := strings.Split(name, "/")
		parent := strings.Join(parts[:len(parts)-1], "/")
		writer.Write([]string{name,
			parent,
			strconv.Itoa(finalTree[name]),
			strconv.Itoa(DenominatorTree[name]),
			strconv.FormatFloat(float64(finalTree[name])/float64(DenominatorTree[name]), 'f', 3, 64),
		})
	}

	writer.Flush()
	f.Close()

}

func regionCounterWorker(taskChan chan Task, wg *sync.WaitGroup) {
	for task := range taskChan {
		bv, err := pp.ReadBVFileToBV(task.Path)
		if err != nil {
			log.Error(err)
			continue
		}

		for i, bit := range bv {
			if bit {
				RegionCountLock.Lock()
				RegionCovCounts[i] += 1
				RegionCountLock.Unlock()
			}
		}

		CompleteCounter += 1
		log.Info(CompleteCounter)
	}
	wg.Done()
}

func writer(writerChan chan Result, outfiledir string, wwg *sync.WaitGroup) {

	for r := range writerChan {

		parts := strings.Split(r.Path, "/")
		rename := parts[len(parts)-4] + "." + parts[len(parts)-3] + ".tree.csv"
		outfileName := path.Join(outfiledir, rename)

		f, err := os.Create(outfileName)
		if err != nil {
			log.Error(err)
			continue
		}

		var keys []string
		for k := range r.FinalTree {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		writer := csv.NewWriter(f)
		writer.Write([]string{"names", "parents", "covered", "total", "percentcovered"})

		for _, name := range keys {
			parts := strings.Split(name, "/")
			parent := strings.Join(parts[:len(parts)-1], "/")
			writer.Write([]string{name,
				parent,
				strconv.Itoa(r.FinalTree[name]),
				strconv.Itoa(DenominatorTree[name]),
				strconv.FormatFloat(float64(r.FinalTree[name])/float64(DenominatorTree[name]), 'f', 3, 64),
			})
		}

		writer.Flush()
		f.Close()
	}

	localIndex := 0
	globalIndex := 0
	medianCoverage := make(map[string]int)
	totalCoverage := make(map[string]int)
	for _, fileName := range SortedFiles {
		medianCoverage[fileName] = 0
		totalCoverage[fileName] = 0
		for _, funcName := range SortedFuncs[fileName] {
			localIndex = 0
			for localIndex < Structure[fileName][funcName] {
				if float64(RegionCovCounts[globalIndex+localIndex]) > float64(TotalBVs)/2.0 {
					medianCoverage[fileName] += 1
				}
				if RegionCovCounts[globalIndex+localIndex] > 0 {
					totalCoverage[fileName] += 1
				}

				localIndex += 1
			}

			globalIndex += Structure[fileName][funcName]
		}
	}

	f, err := os.Create("medianCoverageTree.csv")
	if err != nil {
		log.Error(err)
	}

	finalMedianTree := ConvertFileCoverageToTree(medianCoverage)
	finalTotalTree := ConvertFileCoverageToTree(totalCoverage)

	var keys []string
	for k := range finalMedianTree {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	writer := csv.NewWriter(f)
	writer.Write([]string{"names", "parents", "covered", "total", "percentcovered"})

	for _, name := range keys {
		parts := strings.Split(name, "/")
		parent := strings.Join(parts[:len(parts)-1], "/")
		writer.Write([]string{name,
			parent,
			strconv.Itoa(finalMedianTree[name]),
			strconv.Itoa(DenominatorTree[name]),
			strconv.FormatFloat(float64(finalMedianTree[name])/float64(DenominatorTree[name]), 'f', 3, 64),
		})
	}

	writer.Flush()
	f.Close()

	f, err = os.Create("totalCoverageTree.csv")
	if err != nil {
		log.Error(err)
	}

	writer = csv.NewWriter(f)
	writer.Write([]string{"names", "parents", "covered", "total", "percentcovered"})

	for _, name := range keys {
		parts := strings.Split(name, "/")
		parent := strings.Join(parts[:len(parts)-1], "/")
		writer.Write([]string{strings.Split(name, "/")[len(strings.Split(name, "/"))-1],
			parent,
			strconv.Itoa(finalTotalTree[name]),
			strconv.Itoa(DenominatorTree[name]),
			strconv.FormatFloat(float64(finalTotalTree[name])/float64(DenominatorTree[name]), 'f', 3, 64),
		})
	}

	writer.Flush()
	f.Close()

	wwg.Done()
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
