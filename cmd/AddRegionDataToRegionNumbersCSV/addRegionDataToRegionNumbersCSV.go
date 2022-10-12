package main

import (
	"encoding/csv"
	"flag"
	log "github.com/sirupsen/logrus"
	pp "github.com/teamnsrg/profparse"
	"io"
	"os"
	"sort"
	"strconv"
	"sync"
)

type Task struct {
	Path string
}

type Result struct {
	Path                         string // Done
	Domain                       string // Done
	Category                     string
	Success                      bool  // Done
	TotalResources               int   // Done
	TotalBlocksCovered           int64 // Done
	TotalResourceBytesDownloaded int64 // Done
	TotalScripts                 int64
	TotalDocuments               int64
	TotalImages                  int64
	TotalFonts                   int64
	TotalStylesheets             int64
	TotalXHRs                    int64
	TotalOrigins                 int64
	TotalOriginsScripts          int64
	LoadEvent                    bool             // Done
	LoadEventTime                float64          // Done
	BrowserOpenTime              float64          // Done
	GenBlocksCovered             int64            // Done
	SrcBlocksCovered             int64            // Done
	DirBlocksCovered             map[string]int64 // Done
	PercentDirBlocksCovered      map[string]float64
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

var ExcludeVector []bool

var DirectoriesOfInterest = []string{}

//var DirectoriesOfInterest = []string{
//	"net/websockets",
//}

func main() {
	var covFile string
	var inputFile string
	var outputFile string
	var excludeBVFile string

	flag.StringVar(&covFile, "coverage-file", "coverage.txt",
		"Path to sample text coverage file for metadata generation")
	flag.StringVar(&inputFile, "input-file", "results",
		"Path to MIDA results for analysis")
	flag.StringVar(&outputFile, "output-file", "output/with_region_data_added.csv",
		"Path to output file csv")

	flag.Parse()

	var err error

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

	f, err := os.Open(inputFile)
	if err != nil {
		log.Fatal(err)
	}

	g, err := os.Create(outputFile)
	writer := csv.NewWriter(g)

	header := true
	reader := csv.NewReader(f)
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}

		if err != nil {
			log.Fatal(err)
		}

		if header {
			writer.Write(append(record, []string{"File", "Function"}...))
			header = false
			continue
		}

		regionNum, err := strconv.Atoi(record[0])
		if err != nil {
			log.Fatal(err)
		}

		region := BVIndexToCodeRegionMap[regionNum]
		writer.Write(append(record, []string{
			region.FileName,
			region.FuncName,
		}...))
	}

	writer.Flush()
	f.Close()
	g.Close()

}
