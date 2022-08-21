package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	log "github.com/sirupsen/logrus"
	pp "github.com/teamnsrg/profparse"
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
	Path                         string // Done
	Domain                       string // Done
	Category                     string
	Success                      bool    // Done
	TotalResources               int     // Done
	TotalBlocksCovered           int64   // Done
	TotalResourceBytesDownloaded int64   // Done
	LoadEvent                    bool    // Done
	LoadEventTime                float64 // Done
	BrowserOpenTime              float64 // Done
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

var SiteCats map[string]CloudflareCategoryEntry

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

	log.Info("Loading cloudflare categories...")
	SiteCats, err = LoadCloudflareCategories("/home/pmurley/top1mplusVVNN_categories.json")
	if err != nil {
		log.Fatal(err)
	}
	log.Infof("Loaded %d entries for site categories", len(SiteCats))

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
		pathsParts := strings.Split(task.Path, "/")
		domain := pathsParts[len(pathsParts)-3]

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

		browserOpenedTime := metadata.TaskTiming.BrowserOpen

		loadEventTime := metadata.TaskTiming.LoadEvent
		browserClosedTime := metadata.TaskTiming.BrowserClose

		totalTimeBrowserOpen := browserClosedTime.Sub(browserOpenedTime).Seconds()
		timeToLoadEvent := loadEventTime.Sub(browserOpenedTime).Seconds()

		resourceData, err := pp.LoadMidaResourceData(path.Join(task.Path, "resource_metadata.json"))
		if err != nil {
			log.Error(err)
			continue
		}

		if len(resourceData) < 0 {
			continue
		}

		var blocksCovered int64 = 0
		for _, fileName := range SortedFiles {
			indices := FilenameToBVIndices[fileName]
			fileCov := 0
			for i := indices.Start; i < indices.End; i++ {
				if bv[i] {
					fileCov += 1
					blocksCovered += 1
				}
			}

			FileCovCountLock.Lock()
			if _, ok := FileCoverage[fileName]; !ok {
				FileCoverage[fileName] = make([]float64, 0)
			}
			FileCoverage[fileName] = append(FileCoverage[fileName], float64(fileCov)/float64(indices.End-indices.Start))

			FileCovCountLock.Unlock()
		}

		resourceDir := path.Join(task.Path, "resources")
		dirSize, err := DirSize(resourceDir)
		if err != nil {
			log.Error(err)
		}

		var r Result
		r.Path = task.Path
		r.Domain = domain
		r.Success = metadata.Success
		r.TotalResources = metadata.NumResources
		r.TotalResourceBytesDownloaded = dirSize
		r.TotalBlocksCovered = blocksCovered
		r.BrowserOpenTime = totalTimeBrowserOpen
		if loadEventTime.Year() == 2022 {
			r.LoadEvent = true
			r.LoadEventTime = timeToLoadEvent
		} else {
			r.LoadEvent = false
		}

		if data, ok := SiteCats[domain]; ok && len(data.ContentCategories) == 1 {
			r.Category = data.ContentCategories[0].Name
		} else if len(data.ContentCategories) > 1 {
			r.Category = "MULTIPLE"
		} else {
			r.Category = "UNKNOWN"
		}

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
		"Domain",
		"Path",
		"Category",
		"Success",
		"Load Event Fired",
		"Load Event Time",
		"Browser Open Time",
		"Total Resources",
		"Total Resource Bytes",
		"Total Blocks Covered",
	}

	writer.Write(header)

	for result := range resultChan {
		writer.Write([]string{
			result.Domain,
			result.Path,
			result.Category,
			strconv.FormatBool(result.Success),
			strconv.FormatBool(result.LoadEvent),
			strconv.FormatFloat(result.LoadEventTime, 'f', 2, 64),
			strconv.FormatFloat(result.BrowserOpenTime, 'f', 2, 64),
			strconv.Itoa(result.TotalResources),
			strconv.FormatInt(result.TotalResourceBytesDownloaded, 10),
			strconv.FormatInt(result.TotalBlocksCovered, 10),
		})
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
