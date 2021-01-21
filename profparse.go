package profparse

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
)

/*
func main() {
	err := ReadMapping("mapping.csv")
	if err != nil {
		log.Error(err)
		return
	} else {
		log.Info("Successfully read the mapping")
		log.Infof("%d Functions", len(CovMapping))
		log.Infof("%d Blocks", CovMappingLength)
		log.Infof("Expecting bv files to be %d bytes", CovFileByteLength)
	}

	err = ReadFileMapping("file_mapping.csv")
	if err != nil {
		log.Error(err)
		return
	} else {
		log.Info("Successfully read file mapping")
	}

	log.Info("Calculating number of blocks in each top level dir")
	topLevelBlocks := make(map[string]int)
	for s := range FileMapping {
		topLevelDir := strings.Split(FileMapping[s], "/")[0]

		if _, ok := topLevelBlocks[topLevelDir]; !ok {
			topLevelBlocks[topLevelDir] = 0
		}
	}

	for s := range CovMappingBlockCounts {
		if _, ok := FileMapping[s]; !ok {
			continue
		}
		topLevelDir := strings.Split(FileMapping[s], "/")[0]
		topLevelBlocks[topLevelDir] += CovMappingBlockCounts[s]
	}

	COV_FILE_DIR := "covSamples"

	files, err := ioutil.ReadDir(COV_FILE_DIR)
	if err != nil {
		log.Error(err)
		return
	}

	blockCounts := make([]int, CovMappingLength)
	bvs := make([][]bool,0)

	log.Info("Loading bit vectors...")

	for _, f := range files {
		bv, err := ReadFile(path.Join(COV_FILE_DIR, f.Name()))
		if err != nil {
			log.Error(err)
			return
		}
		for i, val := range bv {
			if val {
				blockCounts[i]++
			}
		}

		bvs = append(bvs, bv)
	}

	log.Infof("Loaded %d bit vectors of coverage data", len(bvs))

	nonZero := 0
	all := 0
	for _, count := range blockCounts {
		if count > 0 {
			nonZero++
		}

		if count == 300 {
			all++
		}
	}

	log.Infof("Altogether, these sites covered %d out of %d blocks (%f percent)",
		nonZero, len(blockCounts), float64(nonZero) * 100.0 / float64(len(blockCounts)))
	log.Infof("%d blocks were covered by every single run (%f percent)", all,
		float64(all) * 100.0 / float64(len(blockCounts)))

	totalBV, _, err := CombineBVs(bvs)
	if err != nil {
		log.Error(err)
		return
	}

	topLevelCovered := make(map[string]int)
	for k := range topLevelBlocks {
		topLevelCovered[k] = 0
	}

	for symbol, fname := range FileMapping {
		if _, ok := CovMappingBlockCounts[symbol]; !ok {
			continue
		}

		numBlocks := CovMappingBlockCounts[symbol]
		startBlock := CovMapping[symbol]
		topLevelDir := strings.Split(fname, "/")[0]

		for i := startBlock; i < startBlock + numBlocks; i++ {
			if totalBV[i] {
				topLevelCovered[topLevelDir]++
			}
		}
	}

	for k := range topLevelCovered {
		fmt.Println(k, ":", topLevelCovered[k], "/", topLevelBlocks[k], "(", math.Round(float64(topLevelCovered[k]) / float64(topLevelBlocks[k]) * 100), "% )")
	}


}


func MergeBVsThreshold(vectors [][]bool, threshold float64) ([]bool, error) {

	// First check and make sure all have the proper length
	for _, v := range vectors {
		if v != nil && len(v) != CovMappingLength {
			log.Error(len(v), " was an unexpected length for BV (expected: ", CovMappingLength, ")")
			return nil, errors.New("improper length bv for combining")
		}
	}

	if threshold > 1.0 || threshold < 0.0 {
		return nil, errors.New("bad threshold")
	}

	counterBV := make([]int, CovMappingLength)
	for _, bv := range vectors {
		for i, bit := range bv {
			if bit {
				counterBV[i] += 1
			}
		}
	}

	var finalBV []bool
	for _, val := range counterBV {
		if float64(val)/float64(len(vectors)) > threshold {
			finalBV = append(finalBV, true)
		} else {
			finalBV = append(finalBV, false)
		}
	}

	return finalBV, nil
}
*/

/*
func CombineBVs(vectors [][]bool) ([]bool, int, error) {
	bv := make([]bool, CovMappingLength)

	// First check and make sure all have the proper length
	for _, v := range vectors {
		if v != nil && len(v) != CovMappingLength {
			return nil, 0, errors.New("improper length bv for combining")
		}
	}

	totalBlocks := 0

	for i := range bv {
		bv[i] = false
		for j := range vectors {
			if vectors[j] == nil {
				continue
			}
			if vectors[j][i] {
				bv[i] = true
				totalBlocks += 1
				break
			}
		}
	}

	return bv, totalBlocks, nil

}
*/

func ReadFileToCovMap(fName string) (map[string]map[string][]bool, error) {

	f, err := os.Open(fName)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	covMap := make(map[string]map[string][]bool)
	currentFile := ""
	currentFunc := ""

	for scanner.Scan() {
		line := scanner.Text()
		pieces := strings.Split(strings.TrimSpace(line), " ")
		if len(pieces) < 2 {
			continue
		}

		if pieces[0] == "[FILE]" {
			currentFile = pieces[1]
			if _, ok := covMap[currentFile]; !ok {
				covMap[currentFile] = make(map[string][]bool)
			}
		} else if pieces[0] == "[FUNCTION]" {
			if currentFile == "" {
				return nil, errors.New("function without a file")
			}

			currentFunc = pieces[1]
			if _, ok := covMap[currentFile][currentFunc]; !ok {
				covMap[currentFile][currentFunc] = make([]bool, 0)
			}
		} else if pieces[0] == "[BLOCK]" {
			if currentFile == "" || currentFunc == "" {
				return nil, errors.New("block without a function or file")
			}

			if len(pieces) != 5 {
				return nil, errors.New("wrong number of pieces in BLOCK line")
			}

			executions, err := strconv.ParseUint(pieces[4], 10, 64)
			if err != nil {
				return nil, err
			}

			if executions != 0 {
				covMap[currentFile][currentFunc] = append(covMap[currentFile][currentFunc], true)
			} else {
				covMap[currentFile][currentFunc] = append(covMap[currentFile][currentFunc], false)
			}

		}
	}

	return covMap, nil
}

func WriteCovMapToFile(fname string, covMap map[string]map[string][]bool) error {

	f, err := os.Create(fname)
	if err != nil {
		return err
	}
	defer f.Close()
	writer := bufio.NewWriter(f)

	fileNames := make([]string, 0, len(covMap))
	for k := range covMap {
		fileNames = append(fileNames, k)
	}
	sort.Strings(fileNames)

	for _, fileName := range fileNames {

		_, err = writer.WriteString("[FILE] " + fileName + "\n")
		if err != nil {
			return err
		}

		funcNames := make([]string, 0, len(covMap[fileName]))
		for k := range covMap[fileName] {
			funcNames = append(funcNames, k)
		}

		sort.Strings(funcNames)

		for _, funcName := range funcNames {
			_, err = writer.WriteString("    " + funcName + " " + strconv.Itoa(len(covMap[fileName][funcName])) + "\n")
		}
	}

	return nil
}

func WriteFileFromBV(fName string, bv []bool) error {
	f, err := os.Create(fName)
	if err != nil {
		return err
	}

	bytesToWrite, err := boolsToBytes(bv)
	if err != nil {
		return err
	}

	numBytes, err := f.Write(bytesToWrite)
	if err != nil {
		return err
	}

	log.Debugf("Wrote %d bytes to file %s", numBytes, fName)

	return nil
}

func ReadBVFileToBV(fname string) ([]bool, error) {
	content, err := ioutil.ReadFile(fname)
	if err != nil {
		return nil, err
	}

	bv, err := bytesToBools(content)
	if err != nil {
		return nil, err
	}

	return bv, nil
}

func ConvertCovMapToBools(covMap map[string]map[string][]bool) []bool {

	bools := make([]bool, 0)
	fileNames := make([]string, 0, len(covMap))
	for k := range covMap {
		fileNames = append(fileNames, k)
	}
	sort.Strings(fileNames)

	for _, fileName := range fileNames {
		funcNames := make([]string, 0, len(covMap[fileName]))
		for k := range covMap[fileName] {
			funcNames = append(funcNames, k)
		}

		sort.Strings(funcNames)

		for _, funcName := range funcNames {
			for _, executions := range covMap[fileName][funcName] {
				if executions {
					bools = append(bools, true)
				} else {
					bools = append(bools, false)
				}
			}
		}
	}

	return bools
}

func boolsToBytes(t []bool) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.LittleEndian, uint32(len(t)))
	if err != nil {
		return nil, err
	}

	b := make([]byte, (len(t)+7)/8)
	for i, x := range t {
		if x {
			b[i/8] |= 0x80 >> uint(i%8)
		}
	}

	b = append(buf.Bytes(), b...)

	log.Infof("Writing a file with %d bytes (%d + %d)", len(b), len(buf.Bytes()), (len(t)+7)/8)

	return b, nil
}

func bytesToBools(b []byte) ([]bool, error) {
	if len(b) < 4 {
		return nil, errors.New("invalid bit vector file")
	}

	buf := bytes.NewReader(b[0:4])

	var numBits uint32
	err := binary.Read(buf, binary.LittleEndian, &numBits)
	if err != nil {
		return nil, err
	}

	t := make([]bool, numBits)
	for i, x := range b[4:] {
		for j := 0; j < 8; j++ {
			if (x<<uint(j))&0x80 == 0x80 {
				t[8*i+j] = true
			}
		}
	}
	return t, nil
}

/*
func ParseFile(fName string) ([]bool, int, error) {
	if CovMappingLength == 0 || len(CovMapping) == 0 {
		return nil, 0, errors.New("cov mapping uninitialized")
	}

	f, err := os.Open(fName)
	if err != nil {
		log.Error(err)
	}

	scanner := bufio.NewScanner(f)

	bv := make([]bool, CovMappingLength) // Number of blocks that we get coverage data for

	currentFunc := ""
	currentIndex := -1

	checker := make(map[int]bool)
	goodFile := false

	lineNum := 0

	blocksCovered := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lineNum += 1
		if strings.HasPrefix(line, "Instrumentation level") {
			goodFile = true
			break
		}

		if strings.HasPrefix(line, "Hash:") {

		} else if strings.HasPrefix(line, "Counters") {

		} else if strings.HasPrefix(line, "Block counts:") {
			arr := strings.TrimSuffix(strings.TrimPrefix(line, "Block counts: ["), "]")
			if arr == "" {
				continue
			}

			blocks := strings.Split(arr, ", ")
			for bIndex, bValStr := range blocks {
				bVal, err := strconv.Atoi(bValStr)
				if err != nil {
					log.Error("Error converting block counter to int")
					log.Error(line)
				}

				bv[currentIndex+1+bIndex] = bVal != 0
				if bVal != 0 {
					blocksCovered += 1
				}
				checker[currentIndex+1+bIndex] = true
			}

		} else if strings.HasPrefix(line, "Function count:") {
			parts := strings.Split(line, " ")
			if len(parts) != 3 {
				log.Error("Bad function count line")
				log.Error(line)
			}
			fCount, err := strconv.Atoi(parts[2])
			if err != nil {
				log.Error("Atoi error in function count")
				log.Error(line)
			}

			bv[currentIndex] = fCount != 0
			if fCount != 0 {
				blocksCovered += 1
			}

			checker[currentIndex] = true
		} else {
			parts := strings.Split(line, " ")
			if len(parts) != 1 {
				log.Error(fName)
				log.Error(line)
				return nil, 0, errors.New("found function line without two parts")
			}
			currentFunc = strings.TrimSuffix(parts[0], ":")

			if _, ok := CovMapping[currentFunc]; !ok {
				log.WithFields(log.Fields{"function": currentFunc, "line": lineNum, "file": fName}).Warn("Missing function")
				currentIndex = -1
			} else {
				currentIndex = CovMapping[currentFunc]
			}
		}
	}

	if !goodFile {
		return nil, 0, errors.New("appears to be a badly formatted file")
	}

	mismatches := 0
	for i := 0; i < CovMappingLength; i++ {
		if _, ok := checker[i]; !ok {
			mismatches += 1
		}
	}

	if mismatches != 0 {
		return nil, 0, errors.New("found mismatches: " + strconv.Itoa(mismatches))
	}

	return bv, blocksCovered, nil
}

*/

/*

// Reads the mapping file and returns the corresponding map structure
func ReadMapping(fname string) error {
	f, err := os.Open(fname)
	if err != nil {
		log.Error(err)
	}

	reader := csv.NewReader(f)

	CovMapping = make(map[string]int)
	CovMappingBlockCounts = make(map[string]int)

	prevSymbol := ""
	prevIndex := 0

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		index, err := strconv.Atoi(row[1])
		if err != nil {
			return err
		}

		if row[0] == "END" {
			CovMappingLength = index
		} else {
			CovMapping[row[0]] = index
		}

		CovMappingBlockCounts[prevSymbol] = index - prevIndex
		prevSymbol = row[0]
		prevIndex = index
	}

	if CovMappingLength%8 == 0 {
		CovFileByteLength = CovMappingLength / 8
	} else {
		CovFileByteLength = CovMappingLength/8 + 1
	}

	return nil
}

func ReadFileMapping(fname string) error {
	f, err := os.Open(fname)
	if err != nil {
		return err
	}

	reader := csv.NewReader(f)

	FileMapping = make(map[string]string)

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		FileMapping[row[0]] = row[1]
	}

	return nil
}

*/

func GetCovPathCrawl(crawlPath string) (string, error) {
	covPath := path.Join(crawlPath, "coverage", "coverage.bv")
	if _, err := os.Stat(covPath); os.IsNotExist(err) {
		return "", errors.New("coverage data does not exist")
	}

	return covPath, nil
}

func GetCovPathsSite(sitePath string) ([]string, error) {
	subDirs, err := ioutil.ReadDir(sitePath)
	if err != nil {
		return nil, err
	}

	result := make([]string, 0)

	for _, sd := range subDirs {
		cp, err := GetCovPathCrawl(path.Join(sitePath, sd.Name()))
		if err != nil {
			log.Error(err)
			continue
		}

		result = append(result, cp)
	}

	if len(result) == 0 {
		return nil, errors.New("no valid coverage data for site")
	}

	return result, nil
}

// Given the path to a MIDA crawl results directory, returns a slice of strings containing
// the paths to all of the coverage (.cov) files contained in it
func GetCovPathsMIDAResults(rootPath string) ([]string, error) {
	results := make([]string, 0)

	dirs, err := ioutil.ReadDir(rootPath)
	if err != nil {
		return nil, err
	}

	for _, site := range dirs {
		paths, err := GetCovPathsSite(path.Join(rootPath, site.Name()))
		if err != nil {
			log.Error(err)
			continue
		}
		results = append(results, paths...)
	}
	return results, nil
}

/*

// Builds a bit vector that is representative of the site's coverage. A site only
// gets credit for covering a block if more than <threshold> percent of the crawls
// covered that block. You may also set a minimum number of crawls for a site, and
// this function will return an error if that minimum is not met.
func BuildRepresentativeBV(sitePath string, threshold float64, minVisits int) ([]bool, error) {
	paths, err := GetCovPathsSite(sitePath)
	if err != nil {
		return nil, err
	}

	if len(paths) < minVisits {
		return nil, errors.New("not enough site visits")
	}

	m := make(map[int]int)

	// Build map containing total number of crawls which cover each block
	for _, p := range paths {
		bv, err := ReadFile(p)
		if err != nil {
			return nil, err
		}

		for i, val := range bv {
			if _, ok := m[i]; !ok {
				m[i] = 0
			}

			if val {
				m[i] += 1
			}
		}
	}

	result := make([]bool, 0)
	for i := 0; i < CovMappingLength; i++ {
		if float64(m[i])/float64(len(paths)) >= threshold {
			result = append(result, true)
		} else {
			result = append(result, false)
		}
	}

	return result, nil
}

func ConvertProfrawsToCovFile(dir string, outputFile string, profdataBinary string) error {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	bvs := make([][]bool, 0)
	for _, cf := range files {
		if !strings.HasSuffix(cf.Name(), "profraw") {
			continue
		}

		fullCovFile := path.Join(dir, cf.Name())
		cmd := exec.Command(profdataBinary, "show", "--counts", "--all-functions", fullCovFile)
		newFileName := strings.ReplaceAll(cf.Name(), "profraw", "txt")
		f, err := os.Create(path.Join(dir, newFileName))
		if err != nil {
			log.Error(err)
			continue
		}
		writer := bufio.NewWriter(f)
		cmd.Stdout = writer
		cmd.Stderr = writer
		err = cmd.Run()
		if err != nil {
			log.Warn(err, "  :  ", fullCovFile)
			continue
		}
		err = writer.Flush()
		if err != nil {
			log.Error(err)
			continue
		}
		err = f.Close()
		if err != nil {
			log.Error(err)
			continue
		}

		fullReport := path.Join(dir, newFileName)
		bv, totalBlocks, err := ParseFile(fullReport)
		if err != nil {
			log.Error(err, " (", fullReport, ")")
			continue
		}

		log.Debugf("%d blocks for %s", totalBlocks, fullCovFile)

		bvs = append(bvs, bv)
	}

	combinedBV, _, err := CombineBVs(bvs)
	err = WriteFile(outputFile, combinedBV)
	if err != nil {
		return err
	}

	return nil
}

*/
