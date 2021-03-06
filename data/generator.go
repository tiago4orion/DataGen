package data

import (
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"encoding/json"
	"github.com/NeowayLabs/clinit-cfn-tool/utils"
	utilsg "github.com/tiago4orion/DataGen/utils"
)

type RecordConfig struct {
	Name  string
	Type  string
	Chars string
	Min   int
	Max   int
}

type DataConfig struct {
	Records    []RecordConfig
	Length     int32
	OutputFile string
	Format     string
}

type WorkerStatus struct {
	Id    int
	Total float64
}

type ProcessWorker struct {
	Id           int
	NRecords     int32
	Config       *DataConfig
	OutputChan   chan interface{}
	WorkStatChan chan WorkerStatus
	wait         *sync.WaitGroup
}

func isWorkersComplete(workStats []float64) bool {
	var complete bool = true

	for i := 0; i < len(workStats); i++ {
		complete = complete && (workStats[i] == 100)
	}

	return complete
}

func CSVLineCreate(record []RecordConfig, r *rand.Rand) []string {
	fields := make([]string, len(record))
	var err error

	for idx, recordField := range record {
		switch recordField.Type {
		case "string":
			fields[idx], err = utilsg.GeneratorString(recordField.Chars,
				recordField.Min, recordField.Max, r)
			if err != nil {
				panic(err)
			}
		case "integer":
			tmpInt, err := utilsg.GeneratorInteger(recordField.Min, recordField.Max, r)
			if err != nil {
				panic(err)
			}

			fields[idx] = strconv.Itoa(tmpInt)
		}
	}

	return fields
}

func JSONCreate(record []RecordConfig, r *rand.Rand) map[string]interface{} {
	var err error

	fields := make(map[string]interface{})

	for _, recordField := range record {
		switch recordField.Type {
		case "string":
			fields[recordField.Name], err = utilsg.GeneratorString(recordField.Chars,
				recordField.Min, recordField.Max, r)

			if err != nil {
				panic(err)
			}
		case "integer":
			tmpInt, err := utilsg.GeneratorInteger(recordField.Min, recordField.Max, r)
			if err != nil {
				panic(err)
			}

			fields[recordField.Name] = strconv.Itoa(tmpInt)
		}
	}

	return fields
}

func processRecords(worker *ProcessWorker) {
	var outLine interface{}

	worker.wait.Add(1)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := int32(0); i < worker.NRecords; i++ {
		if worker.Config.Format == "csv" {
			outLine = CSVLineCreate(worker.Config.Records, r)
		} else if worker.Config.Format == "json" {
			outLine = JSONCreate(worker.Config.Records, r)
		} else {
			panic(errors.New("Invalid format..."))
		}

		worker.OutputChan <- outLine
		worker.WorkStatChan <- WorkerStatus{
			Id:    worker.Id,
			Total: 100.0 * float64(i+1) / float64(worker.NRecords),
		}

		time.Sleep(time.Millisecond)
	}

	defer worker.wait.Done()
}

func outputDataCSV(workerIdx int, outputChan chan interface{}, config *DataConfig, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()
	file, err := os.Create(config.OutputFile + "_" + strconv.Itoa(workerIdx) + ".csv")
	utils.Check(err)

	csvWriter := csv.NewWriter(file)

	for line := range outputChan {
		l := line.([]string)
		if err := csvWriter.Write(l); err != nil {
			panic(err)
		}

		time.Sleep(time.Millisecond)
	}

	csvWriter.Flush()

	defer func() {
		if err := file.Close(); err != nil {
			panic(err)
		}
	}()
}

func outputDataJSON(workerIdx int, outputChan chan interface{}, config *DataConfig, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()
	file, err := os.Create(config.OutputFile + "_" + strconv.Itoa(workerIdx) + ".json")
	utils.Check(err)

	file.Write([]byte("["))

	firstIteration := true

	for doc := range outputChan {
		if !firstIteration {
			file.Write([]byte(","))
		} else {
			firstIteration = false
		}

		d := doc.(map[string]interface{})

		dstr, err := json.Marshal(&d)

		if err != nil {
			panic(err)
		}

		file.Write(dstr)
		time.Sleep(time.Millisecond)
	}

	file.Write([]byte("]"))

	defer func() {
		file.Sync()
		if err := file.Close(); err != nil {
			panic(err)
		}
	}()
}

func workersResumeTotal(workStats []float64) float64 {
	total := float64(0)

	for i := 0; i < len(workStats); i++ {
		total += workStats[i]
	}

	return total / float64(len(workStats))
}

func GenerateData(config *DataConfig, concurrent int, format string) error {
	var wgRecords, wgOutput sync.WaitGroup
	outputChan := make(chan interface{})
	workStatChan := make(chan WorkerStatus)
	ncpu := concurrent

	if concurrent == 0 {
		ncpu = runtime.NumCPU()
	}

	if config.Length < int32(ncpu) {
		ncpu = int(config.Length)
	}

	runtime.GOMAXPROCS(ncpu)

	recordsPerCore := config.Length / int32(ncpu)

	for i := 0; i < ncpu; i++ {
		workerRecords := recordsPerCore
		if i == (ncpu - 1) {
			if float64(config.Length) > float64(ncpu) {
				workerRecords += int32(math.Remainder(float64(config.Length), float64(ncpu)))
			} else {
				workerRecords = int32(config.Length)
			}
		}

		if workerRecords <= 0 {
			continue
		}

		go processRecords(&ProcessWorker{i, workerRecords, config, outputChan, workStatChan, &wgRecords})

		if format == "csv" {
			go outputDataCSV(i, outputChan, config, &wgOutput)
		} else if format == "json" {
			go outputDataJSON(i, outputChan, config, &wgOutput)
		}
	}

	workStats := make([]float64, ncpu)

	for !isWorkersComplete(workStats) {
		fmt.Printf("Workers status: %.2f%%                      \r", workersResumeTotal(workStats))
		select {
		case status := <-workStatChan:
			workStats[status.Id] = status.Total
		}
	}

	fmt.Printf("Workers status: %.2f%%                      \n", workersResumeTotal(workStats))

	close(outputChan)
	wgOutput.Wait()

	return nil
}

func Generator(configFile string, outputFile string, format string, length int32, concurrent int) error {
	var dataConfig DataConfig

	if length == 0 {
		return errors.New("Number of records need be greater than zero.")
	}

	cfgContent := utils.ReadFile(configFile)
	cfgYaml, err := utils.DecodeYaml([]byte(cfgContent))

	utils.Check(err)

	if format == "" {
		tmp, ok := cfgYaml["format"].(string)
		if ok {
			format = tmp
			fmt.Printf("Output format: %s\n", format)
		} else {
			return errors.New("No output format chosen...")
		}
	}

	if outputFile == "" {
		tmp, ok := cfgYaml["filename"].(string)
		if ok {
			outputFile = tmp
			fmt.Printf("Output file: %s\n", outputFile)
		}
	}

	fields := cfgYaml["fields"].([]interface{})
	recordConfig := make([]RecordConfig, len(fields))

	for idx, field := range fields {
		for name, config := range field.(map[interface{}]interface{}) {
			cfg := config.(map[interface{}]interface{})
			chars, ok := cfg["chars"].(string)

			if !ok {
				chars = ""
			}

			rConfig := RecordConfig{
				Name:  name.(string),
				Type:  cfg["type"].(string),
				Chars: chars,
				Min:   cfg["min"].(int),
				Max:   cfg["max"].(int),
			}

			recordConfig[idx] = rConfig
		}
	}

	dataConfig.Records = recordConfig
	dataConfig.Length = length
	dataConfig.OutputFile = outputFile
	dataConfig.Format = format

	err = GenerateData(&dataConfig, concurrent, format)
	return err
}
