package profparse

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
)

type CodeRegion struct {
	fileName    *string
	funcName    *string
	LineStart   int
	ColumnStart int
	LineEnd     int
	ColumnEnd   int
	FileID      int
}

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
			if strings.HasPrefix(currentFile, "/home/pmurley/chromium/src/out/chrome_91_cov_unstripped") {
				currentFile = currentFile[56:]
			}
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

// ReadCovMetadata given the text output by our custom version of llvm-cov, create a metadata object that
// allows us to reason about the bit vectors containing the coverage data
func ReadCovMetadata(fname string) (map[string]map[string][]CodeRegion, error) {

	f, err := os.Open(fname)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	metaMap := make(map[string]map[string][]CodeRegion)
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
			if strings.HasPrefix(currentFile, "/home/pmurley/chromium/src/out/chrome_91_cov_unstripped") {
				currentFile = currentFile[56:]
			}
			if _, ok := metaMap[currentFile]; !ok {
				metaMap[currentFile] = make(map[string][]CodeRegion)
			}
		} else if pieces[0] == "[FUNCTION]" {
			if currentFile == "" {
				return nil, errors.New("function without a file")
			}

			currentFunc = pieces[1]
			if _, ok := metaMap[currentFile][currentFunc]; !ok {
				metaMap[currentFile][currentFunc] = make([]CodeRegion, 0)
			}
		} else if pieces[0] == "[BLOCK]" {
			if currentFile == "" || currentFunc == "" {
				return nil, errors.New("block without a function or file")
			}

			if len(pieces) != 5 {
				return nil, errors.New("wrong number of pieces in BLOCK line")
			}

			var cr CodeRegion
			cr.fileName = &currentFile
			cr.funcName = &currentFunc

			codeIndices := strings.Split(pieces[3], ",")
			if len(codeIndices) != 4 {
				return nil, errors.New("invalid line/column numbers")
			}

			cr.LineStart, err = strconv.Atoi(codeIndices[0])
			if err != nil {
				return nil, errors.New("invalid line/column numbers")
			}

			cr.ColumnStart, err = strconv.Atoi(codeIndices[1])
			if err != nil {
				return nil, errors.New("invalid line/column numbers")
			}

			cr.LineEnd, err = strconv.Atoi(codeIndices[2])
			if err != nil {
				return nil, errors.New("invalid line/column numbers")
			}

			cr.ColumnEnd, err = strconv.Atoi(codeIndices[3])
			if err != nil {
				return nil, errors.New("invalid line/column numbers")
			}

			metaMap[currentFile][currentFunc] = append(metaMap[currentFile][currentFunc], cr)

		}
	}
	return metaMap, nil
}

func ConvertCovMapToStructure(covMap map[string]map[string][]bool) map[string]map[string]int {
	structure := make(map[string]map[string]int)

	for fileName := range covMap {
		structure[fileName] = make(map[string]int)
		for funcName := range covMap[fileName] {
			structure[fileName][funcName] = len(covMap[fileName][funcName])
		}

	}

	return structure
}

func MergeProfraws(profraws []string, outfile string, profdataBinary string, numThreads int) error {
	cmd := exec.Command(profdataBinary, append([]string{"merge",
		"--failure-mode=any", "--num-threads=" + strconv.Itoa(numThreads),
		"--output", outfile}, profraws...)...)

	err := cmd.Run()
	if err != nil {
		return err
	}

	return nil
}

func GenCustomCovTxtFileFromProfdata(profdataFile string, instrumentedBinary string, outfile string, llvmCovBinary string, numThreads int) error {
	f, err := os.Create(outfile)
	if err != nil {
		return err
	}
	defer f.Close()

	cmd := exec.Command(llvmCovBinary, "report",
		"--format=text",
		"--instr-profile="+profdataFile, "-j="+strconv.Itoa(numThreads),
		instrumentedBinary)

	cmd.Stdout = f
	cmd.Stderr = f

	err = cmd.Run()
	if err != nil {
		return err
	}

	return nil
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

func ConvertBoolsToCovMap(bools []bool, structure map[string]map[string]int) (map[string]map[string][]bool, error) {
	covMap := make(map[string]map[string][]bool)

	fileNames := make([]string, 0, len(structure))

	for k := range structure {
		fileNames = append(fileNames, k)
	}
	sort.Strings(fileNames)

	currentIndex := 0

	for _, fileName := range fileNames {

		covMap[fileName] = make(map[string][]bool)

		funcNames := make([]string, 0)
		for k := range structure[fileName] {
			funcNames = append(funcNames, k)
		}

		sort.Strings(funcNames)

		for _, funcName := range funcNames {
			covMap[fileName][funcName] = make([]bool, structure[fileName][funcName])
			for i := 0; i < structure[fileName][funcName]; i++ {
				covMap[fileName][funcName][i] = bools[currentIndex]
				currentIndex += 1
			}
		}
	}

	return covMap, nil
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

type Diff struct {
	TotalRegions      int
	TotalCovered      int
	FirstCovered      int
	SecondCovered     int
	FirstOnlyCovered  int
	SecondOnlyCovered int
	Same              int
	Different         int
}

func DiffTwoCovMaps(c1 map[string]map[string][]bool, c2 map[string]map[string][]bool, filePrefix string) (Diff, error) {
	var d Diff

	for fileName := range c1 {
		if !strings.HasPrefix(fileName, filePrefix) {
			continue
		}

		if _, ok := c2[fileName]; !ok {
			return d, errors.New("mismatched covmaps")
		}

		for funcName := range c1[fileName] {
			if _, ok := c2[fileName][funcName]; !ok {
				return d, errors.New("mismatched covmaps")
			}

			d.TotalRegions += len(c1[fileName][funcName])

			for i, val1 := range c1[fileName][funcName] {
				val2 := c2[fileName][funcName][i]

				if val1 && val2 {
					d.FirstCovered += 1
					d.SecondCovered += 1
					d.Same += 1
					d.TotalCovered += 1

				} else if val1 && !val2 {
					d.FirstCovered += 1
					d.FirstOnlyCovered += 1
					d.Different += 1
					d.TotalCovered += 1

				} else if !val1 && val2 {
					d.SecondCovered += 1
					d.SecondOnlyCovered += 1
					d.Different += 1
					d.TotalCovered += 1

				} else if !val1 && !val2 {
					d.Same += 1
				}
			}
		}

	}

	return d, nil
}

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

// GetMedianBV given a vector of bit vectors, returns a single bit vector best representative of
// that group through simple median
func GetMedianBV(vectors [][]bool) ([]bool, error) {
	if len(vectors) <= 0 {
		return nil, errors.New("no bit vectors passed to GetMedianBV()")
	}

	expectedLength := len(vectors[0])
	if expectedLength == 0 {
		return nil, errors.New("zero length vector in GetMedianBV()")
	}

	finalBV := make([]bool, len(vectors[0]))
	for i := range vectors[0] {
		count := 0
		for j := range vectors {
			if vectors[j][i] {
				count += 1
			}
		}

		if float64(count) >= float64(len(vectors))/2.0 {
			finalBV[i] = true
		} else {
			finalBV[i] = false
		}
	}

	return finalBV, nil
}
