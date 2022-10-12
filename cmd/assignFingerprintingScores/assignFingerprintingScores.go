package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	log "github.com/sirupsen/logrus"
	pp "github.com/teamnsrg/profparse"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type Task struct {
	Path string
}

type Result struct {
	Path  string // Done
	Score float64
}

type BVRange struct {
	Start int //inclusive
	End   int // exclusive
}

type CloudflareContentCategory struct {
	ID              int    `json:"id,omitempty"`
	SuperCategoryId int    `json:"super_category_id,omitempty"`
	Name            string `json:"name,omitempty"`
}

type CloudflareApplication struct {
	ID   int    `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type CloudflareCategoryEntry struct {
	ContentCategories []CloudflareContentCategory `json:"content_categories"`
	Application       CloudflareApplication       `json:"application"`
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

var RegionScores []float64

var SiteCats map[string]CloudflareCategoryEntry

var ExcludeVector []bool

var DirectoriesOfInterest = []string{}

//var DirectoriesOfInterest = []string{
//	"net/websockets",
//}

func main() {
	var covFile string
	var resultsPath string
	var outfile string
	var excludeBVFile string

	flag.StringVar(&covFile, "coverage-file", "coverage.txt",
		"Path to sample text coverage file for metadata generation")
	flag.StringVar(&resultsPath, "results-path", "results",
		"Path to MIDA results for analysis")
	flag.StringVar(&outfile, "out", "output/fingerprinting_scores.csv",
		"Path to output file csv")
	flag.StringVar(&excludeBVFile, "excludeBV", "",
		"BV file to exclude set regions from analysis")

	flag.Parse()

	var err error

	log.Info("Loading cloudflare categories...")
	SiteCats, err = LoadCloudflareCategories("/home/pmurley/top1mplusVVNN_categories.json")
	if err != nil {
		log.Fatal(err)
	}
	log.Infof("Loaded %d entries for site categories", len(SiteCats))

	CompleteCounter = 0
	log.SetReportCaller(true)
	log.Infof("Begin creating metadata structures by reading %s...", covFile)

	var metaProps pp.CovMapProperties
	MetaMap, metaProps, err = pp.ReadCovMetadata(covFile)
	if err != nil {
		log.Fatal(err)
	}

	log.Infof("MetaMap Files: %d, Funcs: %d, Regions: %d", metaProps.NumFiles, metaProps.NumFunctions, metaProps.NumRegions)

	FileCovCounts = make(map[string]int)

	sampleCovMap, props, err := pp.ReadFileToCovMap(covFile)
	if err != nil {
		log.Fatal(err)
	}

	log.Infof("CovMap Files: %d, Funcs: %d, Regions: %d", props.NumFiles, props.NumFunctions, props.NumRegions)

	files := 0
	functions := 0
	regions := 0

	sampleBV := pp.ConvertCovMapToBools(sampleCovMap)

	RegionScores, err = LoadRegionDiffs()
	if err != nil {
		log.Fatal(err)
	}

	log.Info("Reading BV to exclude")
	if excludeBVFile != "" {
		ExcludeVector, err = pp.ReadBVFileToBV(excludeBVFile)
		if err != nil {
			log.Fatal(err)
		}
		log.Infof("Read exclude vector (length: %d)", len(ExcludeVector))
	} else {
		ExcludeVector = make([]bool, len(sampleBV))
		log.Infof("Created empty exclude vector (length: %d)", len(ExcludeVector))
	}

	Structure = pp.ConvertCovMapToStructure(sampleCovMap)
	for _, v1 := range Structure {
		files += 1
		for _, v2 := range v1 {
			functions += 1
			regions += v2
		}
	}
	BVIndexToCodeRegionMap = pp.GenerateBVIndexToCodeRegionMap(Structure, MetaMap)
	log.Infof("BVIndexToCodeRegionMap Length: %d", len(BVIndexToCodeRegionMap))

	// Build FilenameToBVIndices Map
	FilenameToBVIndices = make(map[string]BVRange)
	currentIndex := 0
	start := 0
	end := 0
	totalRegions := 0

	fileNames := make([]string, 0)
	for fileName := range Structure {
		fileNames = append(fileNames, fileName)
	}
	sort.Strings(fileNames)

	for _, fileName := range fileNames {
		start = currentIndex
		funcNames := make([]string, 0)
		for funcName := range Structure[fileName] {
			funcNames = append(funcNames, funcName)
		}

		for _, funcName := range funcNames {
			currentIndex += Structure[fileName][funcName]
		}
		end = currentIndex
		FilenameToBVIndices[fileName] = BVRange{
			Start: start,
			End:   end,
		}
		totalRegions += end - start
	}

	log.Infof("Total Files, Regions for FilenameToBVIndices: %d, %d", len(FilenameToBVIndices), totalRegions)

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
	resultsChan := make(chan Result, 10000)
	var wg sync.WaitGroup
	var wwg sync.WaitGroup

	wwg.Add(1)
	go writer(resultsChan, &wwg, outfile)

	WORKERS := 28
	for i := 0; i < WORKERS; i++ {
		wg.Add(1)
		go worker(taskChan, resultsChan, &wg)
	}

	for _, resultsPath := range resultsPaths {
		var t Task
		t.Path = resultsPath
		taskChan <- t
	}

	close(taskChan)
	wg.Wait()
	log.Info("Workers finished, starting")

	close(resultsChan)
	wwg.Wait()

	log.Info("Writer has completed")

	numTrials := len(covPaths)
	log.Infof("numTrials: %d", numTrials)
}

func worker(taskChan chan Task, resultChan chan Result, wg *sync.WaitGroup) {
	for task := range taskChan {
		log.Infof("Processing task: %s", task.Path)

		bv, err := pp.ReadBVFileToBV(path.Join(task.Path, "coverage", "coverage.bv"))
		if err != nil {
			log.Error(err)
			continue
		}

		metadata, err := pp.LoadMidaMetadata(path.Join(task.Path, "metadata.json"))
		if err != nil {
			log.Error(err)
			continue
		}

		if !metadata.Success {
			continue
		}

		var r Result

		score := 0.00

		for i := range bv {
			if ExcludeVector[i] {
				continue
			}

			if bv[i] {
				score += RegionScores[i]
			}
		}

		r.Path = task.Path
		r.Score = score

		resultChan <- r

		CompleteCounter += 1
		log.Info(CompleteCounter)
	}
	wg.Done()
}

func writer(resultChan chan Result, wwg *sync.WaitGroup, outfile string) {

	f, err := os.Create(outfile)
	if err != nil {
		log.Fatal(err)
	}

	writer := csv.NewWriter(f)

	header := []string{
		"Path",
		"Score",
	}

	writer.Write(header)

	for result := range resultChan {
		line := []string{
			result.Path,
			strconv.FormatFloat(result.Score, 'f', 8, 64),
		}

		writer.Write(line)
		writer.Flush()
	}
	f.Close()
	wwg.Done()
}

func DirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size, err
}

func LoadCloudflareCategories(filename string) (map[string]CloudflareCategoryEntry, error) {
	jsonBytes, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var entry map[string]CloudflareCategoryEntry
	err = json.Unmarshal(jsonBytes, &entry)
	if err != nil {
		return nil, err
	}

	return entry, nil
}

// Returns an array where the index is the region number
func LoadRegionDiffs() ([]float64, error) {
	result := make([]float64, 6389348)

	for i := range result {
		result[i] = 0.00000
	}

	f, err := os.Open("output/differences_in_frequency.csv")
	if err != nil {
		log.Fatal(err)
	}

	reader := csv.NewReader(f)
	header := true
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}

		if err != nil {
			log.Fatal(err)
		}

		if header {
			header = false
			continue
		}

		regionNum, err := strconv.Atoi(record[0])
		if err != nil {
			log.Fatal(err)
		}

		percent, err := strconv.ParseFloat(record[7], 64)
		percentNegative, err := strconv.ParseFloat(record[6], 64)
		percentPositive, err := strconv.ParseFloat(record[3], 64)

		if percentPositive > 0.50 && percentNegative < 0.05 {
			percent = 1.0
		} else {
			percent = 0
		}

		if err != nil {
			log.Fatal(err)
		}

		result[regionNum] = percent

	}

	return result, nil
}
