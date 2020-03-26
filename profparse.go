package profparse

import (
	"bufio"
	"encoding/csv"
	"errors"
	log "github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
)
/*
func main() {
	log.Info("Start")

	blockMap := ReadMapping("mapping.csv")

	bv, err := ParseFile("report.txt", blockMap)
	if err != nil {
		log.Error(err)
	}
	bv2, err := ParseFile("report2.txt", blockMap)
	if err != nil {
		log.Error(err)
	}
	bv3, err := ParseFile("report3.txt", blockMap)
	if err != nil {
		log.Error(err)
	}
	bv4, err := ParseFile("report4.txt", blockMap)
	if err != nil {
		log.Error(err)
	}
	bv5, err := ParseFile("report5.txt", blockMap)
	if err != nil {
		log.Error(err)
	}

	oneTrues := 0
	twoTrues := 0
	threeTrues := 0
	fourTrues := 0
	fiveTrues := 0
	combinedTrues := 0

	for _, val := range bv {
		if val {
			oneTrues += 1
		}
	}

	for _, val := range bv2 {
		if val {
			twoTrues += 1
		}
	}
	for _, val := range bv3 {
		if val {
			threeTrues += 1
		}
	}
	for _, val := range bv4 {
		if val {
			fourTrues += 1
		}
	}
	for _, val := range bv5 {
		if val {
			fiveTrues += 1
		}
	}

	bvCombined, err := CombineBVs([][]bool{bv, bv2, bv3, bv4, bv5})
	if err != nil {
		log.Error(err)
	}

	for _, val := range bvCombined {
		if val {
			combinedTrues += 1
		}
	}

	log.Infof("bv: %d out of %d", oneTrues, len(bv))
	log.Infof("bv2: %d out of %d", twoTrues, len(bv2))
	log.Infof("bv3: %d out of %d", threeTrues, len(bv3))
	log.Infof("bv4: %d out of %d", fourTrues, len(bv4))
	log.Infof("bv5: %d out of %d", fiveTrues, len(bv5))
	log.Infof("bvCombined: %d out of %d", combinedTrues, len(bvCombined))

	log.Info("End")
}
*/


func CombineBVs(vectors [][]bool) ([]bool, int,  error) {
	bv := make([]bool, 5307107)

	// First check and make sure all have the proper length
	for _, v := range vectors {
		if v != nil && len(v) != 5307107 {
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

	if len(bytes) != 708486 {
		log.Error("Wrong number of bytes read")
		log.Error("708486 != ", len(bytes))
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
	if numBytes != 708486 {
		log.Warnf("Warning: Wrote unexpected number of bytes (708486 expected, %d written)", numBytes)
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
	return t[:len(t) - 3]
}

func ParseFile(fName string, mapping *map[string]int) ([]bool, int, error) {
	f, err := os.Open(fName)
	if err != nil {
		log.Error(err)
	}

	scanner := bufio.NewScanner(f)

	bv := make([]bool, 5307107) // Number of blocks that we get coverage data for

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

				bv[currentIndex + 1 + bIndex] = bVal != 0
				if bVal != 0 {
					blocksCovered += 1
				}
				checker[currentIndex + 1 + bIndex] = true
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
			parts  := strings.Split(line, " ")
			if len(parts) != 1 {
				log.Error(fName)
				log.Error(line)
				return nil, 0, errors.New("found function line without two parts")
			}
			currentFunc = strings.TrimSuffix(parts[0],":")

			if _, ok := (*mapping)[currentFunc]; !ok {
				log.WithFields(log.Fields{"function":currentFunc, "line": lineNum, "file": fName}).Warn("Missing function")
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
	for i :=0; i<5307107; i++ {
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


