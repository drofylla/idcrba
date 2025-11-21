package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ebfe/scard"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context
}

type MyKadData struct {
	Name         string `json:"name"`
	ICNumber     string `json:"ic_number"`
	Sex          string `json:"sex"`
	DOB          string `json:"date_of_birth"`
	StateOfBirth string `json:"state_of_birth"`
	Address1     string `json:"address1"`
	Address2     string `json:"address2"`
	Address3     string `json:"address3"`
	Postcode     string `json:"postcode"`
	City         string `json:"city"`
	Religion     string `json:"religion"`
	ReadTime     string `json:"read_time"`
}

type ReaderStatus struct {
	Connected bool   `json:"connected"`
	Reader    string `json:"reader"`
	Message   string `json:"message"`
	HasCard   bool   `json:"has_card"`
}

// Const data offsets for MyKad
const (
	offsetName       = 233
	lengthName       = 40
	offsetIC         = 273
	lengthIC         = 13
	offsetSex        = 300
	lengthSex        = 1
	offsetDOB        = 301
	lengthDOB        = 10
	offsetStateBirth = 312
	lengthStateBirth = 25
	offsetAddr1      = 411
	lengthAddr1      = 30
	offsetAddr2      = 441
	lengthAddr2      = 30
	offsetAddr3      = 471
	lengthAddr3      = 30
	offsetPostcode   = 574
	lengthPostcode   = 5
	offsetCity       = 579
	lengthCity       = 25
	offsetReligion   = 653
	lengthReligion   = 10
)

var (
	apduSelectJPN = []byte{
		0x00, 0xA4, 0x04, 0x00, 0x0B, 0x68, 0x04, 0x00, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01,
	}
	appState = &AppState{}
)

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	log.Println("IDCRBA started")

	go a.startAutoReadMonitor()
}

//SCARD FUNCTIONS

func sendAPDU(card *scard.Card, cmd []byte) ([]byte, error) {
	res, err := card.Transmit(cmd)
	if err != nil {
		return nil, fmt.Errorf("Failed to transmit APDU: %v", err)
	}

	if len(res) < 2 {
		return nil, fmt.Errorf("Invalid APDU response: %X", res)
	}

	status := uint16(res[len(res)-2])<<8 | uint16(res[len(res)-1])
	if status != 0x9000 {
		return nil, fmt.Errorf("card returned error status: %X", status)
	}

	return res[:len(res)-2], nil
}

func readData(card *scard.Card, offset uint16, length byte) ([]byte, error) {
	offsetHigh := byte(offset >> 8)
	offsetLow := byte(offset & 0xFF)
	cmd := []byte{0x00, 0xB0, offsetHigh, offsetLow, length}

	res, err := sendAPDU(card, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to read data at offset %d: %v", offset, err)
	}
	return res, nil
}

func cleanString(data []byte) string {
	return string.TrimRight(stirng(data), "\x00 ")
}

//DM FUNCTIONS

func (a *App) SaveDataToJson(data MyKadData) (string, error) {
	if data.ICNumber == "" {
		return "", fmt.Errorf("unable to save file, IC number is empty")
	}

	safeIC := strings.ReplaceAll(data.ICNumber, "/", "-")
	safeIC = strings.ReplaceAll(safeIC, "\\", "-")

	const dataDir = "mykad_data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return "", fmt.Errorf("unable to create data directory: %v", err)
	}

	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s_%s.json", safeIC, timestamp)
	filePath := filepath.Join(dataDir, filename)

	jsonData, err := json.MarshalIndent(data, "", " ")
	if err != nil {
		return "", fmt.Errorf("unable to marshal data to JSON: %w", err)
	}

	if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
		return "", fmt.Errorf("unable to write JSON file: %w", err)
	}

	log.Printf("Data saved to: %s", filePath)
	return filePath, nil
}

func (a *App) CheckReaderStatus() ReaderStatus {
	context, err := scard.EstablishContext()
	if err != nil {
		return ReaderStatus{
			Connected: false,
			Message:   fmt.Sprintf("Failed to establish context: %v", err),
			HasCard:   false,
		}
	}
	defer context.Release()

	readers, err := context.ListReaders()
	if err != nil {
		return ReaderStatus{
			Connected: false,
			Message:   fmt.Sprintf("Failed to list readers: %v", err),
			HasCard:   false,
		}
	}
	if len(readers) == 0 {
		return ReaderStatus{
			Connected: false,
			Message:   "No smart card readers found",
			HasCard:   false,
		}
	}

	card, err := context.Connect(readers[0], scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		return ReaderStatus{
			Connected: true,
			Reader:    readers[0],
			Message:   "Reader ready - please insert MyKad",
			HasCard:   false,
		}
	}
	defer card.Disconnect(scard.LeaveCard)

	return ReaderStatus{
		Connected: true,
		Reader:    readers[0],
		Message:   "MyKad detected and ready to read",
		HasCard:   true,
	}
}

func (a *App) ReadCardData() (*MyKadData, err) {
	context, err := scard.EstablishContext()
	if err != nil {
		return nil, fmt.Errorf("unable to establish context: %v", err)
	}
	defer context.Release()

	readers, err := context.ListReaders()
	if err != nil {
		return nil, fmt.Errorf("failed to list readers: %v", err)
	}
	if len(readers) == 0 {
		return nil, fmt.Errorf("no smart card readers found")
	}

	card, err := context.Connect(readers[0], scard.ShareShared, scard.ProtocolAny)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to reader: %v", err)
	}
	defer card.Disconnect(scard.LeaveCard)

	//select jpn app
	_, err = sendAPDU(card, apduSelectJPN)
	if err != nil {
		return nil, fmt.Errorf("unable to select JPN application: %v", err)
	}

	var data MyKadData

	bName, err := readData(card, offsetName, lengthName)
	if err == nil {
		data.Name = cleanString(bName)
	}

	bIC, err := readData(card, offsetIC, lengthIC)
	if err == nil {
		data.ICNumber = cleanString(bIC)
	}

	bSex, err := readData(card, offsetSex, lengthSex)
	if err == nil {
		data.Sex = cleanString(bSex)
	}

	bDOB, err := readData(card, offsetDOB, lengthDOB)
	if err == nil {
		data.DOB = cleanString(bDOB)
	}

	bStateBirth, err := readData(card, offsetStateBirth, lengthStateBirth)
	if err == nil {
		data.StateOfBirth = cleanString(bStateBirth)
	}

	bAddr1, err := readData(card, offsetAddr1, lengthAddr1)
	if err == nil {
		data.Address1 = cleanString(bAddr1)
	}

	bAddr2, err := readData(card, offsetAddr2, lengthAddr2)
	if err == nil {
		data.Address2 = cleanString(bAddr2)
	}

	bAddr3, err := readData(card, offsetAddr3, lengthAddr3)
	if err == nil {
		data.Address3 = cleanString(bAddr3)
	}

	bPostcode, err := readData(card, offsetPostcode, lengthPostcode)
	if err == nil {
		data.Postcode = cleanString(bPostcode)
	}

	bCity, err := readData(card, offsetCity, lengthCity)
	if err == nil {
		data.City = cleanString(bCity)
	}

	bReligion, err := readData(card, offsetReligion, lengthReligion)
	if err == nil {
		data.Religion = cleanString(bReligion)
	}

	data.ReadTime = time.Now().Format("2006-01-02 15:04:05")

	return &data, nil
}

func (a *App) ReadAndSaveCardData() (*MyKadData, string, error) {
	data, err := a.ReadCardData()
	if err != nil {
		return nil, "", err
	}

	filePath, err := a.SaveDataToJSON(*data)
	if err != nil {
		return data, "", err
	}

	return data, filePath, nil
}

// frontend exposed functions

func (a *App) GetReaderStatus() ReaderStatus {
	return a.CheckReaderStatus()
}

func (a *App) ReadMyKad() (string, error) {
	appState.Lock()
	appState.IsReading = true
	appState.Unlock()

	defer func() {
		appState.Lock()
		appState.IsReading = false
		appState.Unlock()
	}()

	data, filePath, err := a.ReadAndSaveCardData()
	if err != nil {
		return "", err
	}

	appState.Lock()
	appState.CurrentData = data
	appState.LastReadTime = time.Now()
	appState.Unlock()

	result := map[string]interface{}{
		"data":     data,
		"filePath": filePath,
		"success":  true,
	}

	jsonData, err := json.Marshal(result)
	if err != nil {
		return "", err
	}

	return string(jsonData), nil
}

func (a *App) GetLatestData() (string, error) {
	appState.RLock()
	defer appState.RUnlock()

	if appState.CurrentData == nil {
		return `{"error": "No data available"}`, nil
	}

	result := map[string]interface{}{
		"data":     appState.CurrentData,
		"readTime": appState.LastReadTime.Format("2006-01-02 15:04:05"),
		"hasData":  true,
	}

	jsonData, err := json.Marshal(result)
	if err != nil {
		return "", err
	}

	return string(jsonData), nil
}

func (a *App) SetAutoRead(enabled bool) {
	appState.Lock()
	appState.AutoRead = enabled
	appState.Unlock()
}

func (a *App) GetAutoReadStatus() bool {
	appState.RLock()
	defer appState.RUnlock()
	return appState.AutoRead
}

//Auto read screen

func (a *App) startAutoReadMonitor() {
	var lastCardState bool = false

	for {
		// Check if auto-read is enabled
		appState.RLock()
		autoReadEnabled := appState.AutoRead
		isReading := appState.IsReading
		appState.RUnlock()

		if !autoReadEnabled {
			time.Sleep(2 * time.Second)
			continue
		}

		status := a.CheckReaderStatus()

		// Card inserted
		if status.HasCard && !lastCardState && !isReading {
			log.Println("MyKad detected! Auto-reading data...")

			// Read the card data
			data, filePath, err := a.ReadAndSaveCardData()
			if err != nil {
				log.Printf("Error auto-reading card: %v", err)
			} else {
				appState.Lock()
				appState.CurrentData = data
				appState.LastReadTime = time.Now()
				appState.ReaderName = status.Reader
				appState.Unlock()

				log.Printf("Auto-read successful! Saved to: %s", filePath)

				// Notify frontend
				runtime.EventsEmit(a.ctx, "autoReadCompleted", map[string]interface{}{
					"data":     data,
					"filePath": filePath,
				})
			}
		}

		// Card removed
		if !status.HasCard && lastCardState {
			log.Println("MyKad removed")
			appState.Lock()
			appState.CurrentData = nil
			appState.Unlock()

			// Notify frontend
			runtime.EventsEmit(a.ctx, "cardRemoved", nil)
		}

		lastCardState = status.HasCard
		time.Sleep(1 * time.Second) // Check every second
	}
}
