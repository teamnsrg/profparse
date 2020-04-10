package profparse

import (
	"bufio"
	"encoding/csv"
	"errors"
	log "github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
)

const BV_LENGTH = 5307106
const BV_FILE_BYTES = 66666 // TODO

/*
func main() {


	if len(os.Args) != 3 {
		log.Fatal("Usage: ./pp <mida_results> <output_file>")
	}

	paths, err := ioutil.ReadDir(os.Args[1])
	if err != nil {
		log.Error(err)
		return
	}

	m := make(map[string][]bool)

	for _, sitePath := range paths {
		bv, err := BuildRepresentativeBV(path.Join(os.Args[1], sitePath.Name()), 0.7, 4)
		if err != nil {
			log.Error(err)
			return
		}

		m[path.Join(os.Args[1], sitePath.Name())] = bv
	}

	orderedPaths, orderedBlocks, err := FastGreedy(&m,2)

	f, err := os.Create(os.Args[2])
	writer := csv.NewWriter(f)

	total := 0
	for i := range orderedPaths {
		log.Infof("%d ( %s )", orderedBlocks[i], orderedPaths[i])
		err = writer.Write([]string{orderedPaths[i],strconv.Itoa(orderedBlocks[i]), strconv.Itoa(total)})
		if err != nil {
			log.Error(err)
		}
		writer.Flush()
	}

	err = f.Close()
	if err != nil {
		log.Error(err)
	}

	return
}
*/

func CombineBVs(vectors [][]bool) ([]bool, int, error) {
	bv := make([]bool, BV_LENGTH)

	// First check and make sure all have the proper length
	for _, v := range vectors {
		if v != nil && len(v) != BV_LENGTH {
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

func ReadFile(fName string) ([]bool, error) {
	f, err := os.Open(fName)
	if err != nil {
		return nil, err
	}

	bytes, err := ioutil.ReadAll(f)
	if err != nil {
		log.Error(err)
	}

	if len(bytes) != BV_FILE_BYTES {
		log.Error("Wrong number of bytes read")
		log.Errorf("%d != %d", BV_FILE_BYTES, len(bytes))
		return nil, err
	}

	return bytesToBools(bytes), nil
}

func WriteFile(fName string, bv []bool) error {
	f, err := os.Create(fName)
	if err != nil {
		return err
	}

	numBytes, err := f.Write(boolsToBytes(bv))
	if err != nil {
		return err
	}

	log.Infof("Wrote %d bytes to file %s", numBytes, fName)
	if numBytes != BV_FILE_BYTES {
		log.Warnf("Warning: Wrote unexpected number of bytes (%d expected, %d written)", BV_FILE_BYTES, numBytes)
	}

	return nil
}

func boolsToBytes(t []bool) []byte {
	b := make([]byte, (len(t)+7)/8)
	for i, x := range t {
		if x {
			b[i/8] |= 0x80 >> uint(i%8)
		}
	}
	return b
}

func bytesToBools(b []byte) []bool {
	t := make([]bool, 8*len(b))
	for i, x := range b {
		for j := 0; j < 8; j++ {
			if (x<<uint(j))&0x80 == 0x80 {
				t[8*i+j] = true
			}
		}
	}
	return t[:len(t)-3]
}

func ParseFile(fName string, mapping *map[string]int) ([]bool, int, error) {
	f, err := os.Open(fName)
	if err != nil {
		log.Error(err)
	}

	scanner := bufio.NewScanner(f)

	bv := make([]bool, BV_LENGTH) // Number of blocks that we get coverage data for

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

			if _, ok := (*mapping)[currentFunc]; !ok {
				log.WithFields(log.Fields{"function": currentFunc, "line": lineNum, "file": fName}).Warn("Missing function")
				currentIndex = -1
			} else {
				currentIndex = (*mapping)[currentFunc]
			}
		}
	}

	if !goodFile {
		return nil, 0, errors.New("appears to be a badly formatted file")
	}

	mismatches := 0
	for i := 0; i < 5667885; i++ {
		if _, ok := checker[i]; !ok {
			mismatches += 1
		}
	}

	if mismatches != 0 {
		return nil, 0, errors.New("found mismatches: " + strconv.Itoa(mismatches))
	}

	return bv, blocksCovered, nil
}

// Reads the mapping file and returns the corresponding map structure
func ReadMapping(fname string) *map[string]int {
	f, err := os.Open(fname)
	if err != nil {
		log.Error(err)
	}

	fMap := make(map[string]int)

	reader := csv.NewReader(f)

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			log.Error(err)
			break
		}

		index, err := strconv.Atoi(row[1])
		if err != nil {
			log.Error(err)
			break
		}

		fMap[row[0]] = index
	}

	return &fMap
}

func GetCovPathCrawl(crawlPath string) (string, error) {
	covPath := path.Join(crawlPath, "coverage", "coverage.cov")
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

func FriendlyGreedy(paths []string, rounds int) ([]string, []int, error) {

	// Get the length from the first entry in our paths. Every other one must match
	// this length
	bv, err := ReadFile(paths[0])
	if err != nil {
		return nil, nil, err
	}

	bvLength := len(bv)

	// Blocks covered so far
	covered := make(map[int]bool)

	// Keys (paths to cov files) already selected in previous rounds
	used := make(map[string]bool)

	// We count down
	roundsRemaining := rounds

	var orderedPaths []string
	var orderedNumBlocks []int

	for roundsRemaining != 0 && len(covered) != bvLength {

		log.Infof("Rounds Remaining: %d", roundsRemaining)
		log.Infof("Blocks covered: %d", len(covered))
		log.Infof("Used: %d", len(used))

		bestCandidate := ""
		bestCandidateNewCovered := 0

		for _, bvPath := range paths {

			bv, err := ReadFile(bvPath)
			if err != nil {
				log.Error(err, " : ", bvPath)
				continue
			}

			// Skip over already used bvs
			if _, ok := used[bvPath]; ok {
				continue
			}

			newBlocks := 0
			for i, val := range bv {
				if _, ok := covered[i]; !ok && val {
					newBlocks += 1
				}
			}

			if newBlocks > bestCandidateNewCovered {
				// We found a new best candidate
				bestCandidate = bvPath
				bestCandidateNewCovered = newBlocks
			}

		}

		if bestCandidateNewCovered == 0 {
			log.Info("Exiting greedy function as have no remaining candidate which covers new blocks")
			break
		}

		// We have successfully selected our next candidate, add it to our results and update data
		// structures accordingly
		orderedPaths = append(orderedPaths, bestCandidate)
		orderedNumBlocks = append(orderedNumBlocks, bestCandidateNewCovered)

		winnerBV, err := ReadFile(bestCandidate)
		if err != nil {
			return nil, nil, err
		}
		for i, val := range winnerBV {
			if val {
				covered[i] = true
			}
		}

		used[bestCandidate] = true

		roundsRemaining -= 1
	}

	return orderedPaths, orderedNumBlocks, nil

}

// Takes a mapping of file paths to coverage bit vectors. Executes a greedy algorithm
// and returns an ordered list of file paths, along with the coverage you get for each.
// Pass -1 for rounds to do as many rounds as needed to cover all observed blocks
func FastGreedy(bvs *map[string][]bool, rounds int) ([]string, []int, error) {
	bvLength := -1

	// Verify that each bit vector is the same the length
	for _, bv := range *bvs {
		if bvLength != -1 && bvLength != len(bv) {
			return nil, nil, errors.New("different bit vector lengths")
		}
		bvLength = len(bv)
	}

	// Blocks covered so far
	covered := make(map[int]bool)

	// Keys (paths to cov files) already selected in previous rounds
	used := make(map[string]bool)

	// We count down
	roundsRemaining := rounds

	var orderedPaths []string
	var orderedNumBlocks []int

	for roundsRemaining != 0 && len(covered) != bvLength {
		log.Infof("Rounds Remaining: %d", roundsRemaining)
		log.Infof("Blocks covered: %d", len(covered))
		log.Infof("Used: %d", len(used))
		bestCandidate := ""
		bestCandidateNewCovered := 0

		for k, bv := range *bvs {

			// Skip over already used bvs
			if _, ok := used[k]; ok {
				continue
			}

			newBlocks := 0
			for i, val := range bv {
				if _, ok := covered[i]; !ok && val {
					newBlocks += 1
				}
			}

			if newBlocks > bestCandidateNewCovered {
				// We found a new best candidate
				bestCandidate = k
				bestCandidateNewCovered = newBlocks
			}

		}

		if bestCandidateNewCovered == 0 {
			log.Info("Exiting greedy function as have no remaining candidate which covers new blocks")
			break
		}

		// We have successfully selected our next candidate, add it to our results and update data
		// structures accordingly
		orderedPaths = append(orderedPaths, bestCandidate)
		orderedNumBlocks = append(orderedNumBlocks, bestCandidateNewCovered)

		for i, val := range (*bvs)[bestCandidate] {
			if val {
				covered[i] = true
			}
		}

		used[bestCandidate] = true

		roundsRemaining -= 1
	}

	return orderedPaths, orderedNumBlocks, nil
}

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
	for i := 0; i < BV_LENGTH; i++ {
		if float64(m[i])/float64(len(paths)) >= threshold {
			result = append(result, true)
		} else {
			result = append(result, false)
		}
	}

	return result, nil
}

func ConvertProfrawsToCov(dir string, outputFile string, profdataBinary string, mapping *map[string]int) error {
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
			log.Error(err, "  :  ", fullCovFile)
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
		bv, totalBlocks, err := ParseFile(fullReport, mapping)
		if err != nil {
			log.Error(err, fullReport)
			continue
		}

		log.Infof("%d blocks for %s", totalBlocks, fullCovFile)

		bvs = append(bvs, bv)
	}

	combinedBV, _, err := CombineBVs(bvs)
	err = WriteFile(outputFile, combinedBV)
	if err != nil {
		return err
	}

	return nil
}
