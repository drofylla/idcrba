package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ebfe/scard"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)


var assets embed.FS

// ============================
// DATA STRUCTURES
// ============================

type MyKadData struct {
	Name         string `json:"name"`
	ICNumber     string `json:"ic_number"`
	Sex          string `json:"sex"`
	DOB          string `json:"date_of_birth"`
	StateOfBirth string `json:"state_of_birth"`
	Address1     string `json:"address_1"`
	Address2     string `json:"address_2"`
	Address3     string `json:"address_3"`
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

type AppState struct {
	sync.RWMutex
	CurrentData  *MyKadData `json:"current_data"`
	LastReadTime time.Time  `json:"last_read_time"`
	IsReading    bool       `json:"is_reading"`
	ReaderName   string     `json:"reader_name"`
	AutoRead     bool       `json:"auto_read"`
}

// ============================
// CONSTANTS AND GLOBALS
// ============================

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

// ============================
// APP STRUCT AND METHODS
// ============================

type App struct {
	ctx context.Context
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	log.Println("MyKad Reader Desktop App started")
	go a.startAutoReadMonitor()
}

// ============================
// SCARD FUNCTIONS
// ============================

func (a *App) sendAPDU(card *scard.Card, cmd []byte) ([]byte, error) {
	res, err := card.Transmit(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to transmit APDU: %v", err)
	}

	if len(res) < 2 {
		return nil, fmt.Errorf("invalid APDU response: %X", res)
	}

	status := uint16(res[len(res)-2])<<8 | uint16(res[len(res)-1])
	if status != 0x9000 {
		return nil, fmt.Errorf("card returned error status: %X", status)
	}

	return res[:len(res)-2], nil
}

func (a *App) readData(card *scard.Card, offset uint16, length byte) ([]byte, error) {
	offsetHigh := byte(offset >> 8)
	offsetLow := byte(offset & 0xFF)
	cmd := []byte{0x00, 0xB0, offsetHigh, offsetLow, length}

	res, err := a.sendAPDU(card, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to read data at offset %d: %v", offset, err)
	}
	return res, nil
}

func (a *App) cleanString(data []byte) string {
	return strings.TrimRight(string(data), "\x00 ")
}

// ============================
// DATA MANAGEMENT FUNCTIONS
// ============================

func (a *App) SaveDataToJSON(data MyKadData) (string, error) {
	if data.ICNumber == "" {
		return "", fmt.Errorf("cannot save file, IC number is empty")
	}

	safeIC := strings.ReplaceAll(data.ICNumber, "/", "-")
	safeIC = strings.ReplaceAll(safeIC, "\\", "-")

	const dataDir = "mykad_data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create data directory: %w", err)
	}

	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s_%s.json", safeIC, timestamp)
	filePath := filepath.Join(dataDir, filename)

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal data to JSON: %w", err)
	}

	if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
		return "", fmt.Errorf("failed to write JSON file: %w", err)
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
			Message:   "Reader ready - waiting for MyKad",
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

func (a *App) ReadCardData() (*MyKadData, error) {
	context, err := scard.EstablishContext()
	if err != nil {
		return nil, fmt.Errorf("failed to establish context: %v", err)
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
		return nil, fmt.Errorf("failed to connect to reader: %v", err)
	}
	defer card.Disconnect(scard.LeaveCard)

	_, err = a.sendAPDU(card, apduSelectJPN)
	if err != nil {
		return nil, fmt.Errorf("failed to select JPN application: %v", err)
	}

	var data MyKadData

	bName, err := a.readData(card, offsetName, lengthName)
	if err == nil {
		data.Name = a.cleanString(bName)
	}

	bIC, err := a.readData(card, offsetIC, lengthIC)
	if err == nil {
		data.ICNumber = a.cleanString(bIC)
	}

	bSex, err := a.readData(card, offsetSex, lengthSex)
	if err == nil {
		data.Sex = a.cleanString(bSex)
	}

	bDOB, err := a.readData(card, offsetDOB, lengthDOB)
	if err == nil {
		data.DOB = a.cleanString(bDOB)
	}

	bStateBirth, err := a.readData(card, offsetStateBirth, lengthStateBirth)
	if err == nil {
		data.StateOfBirth = a.cleanString(bStateBirth)
	}

	bAddr1, err := a.readData(card, offsetAddr1, lengthAddr1)
	if err == nil {
		data.Address1 = a.cleanString(bAddr1)
	}

	bAddr2, err := a.readData(card, offsetAddr2, lengthAddr2)
	if err == nil {
		data.Address2 = a.cleanString(bAddr2)
	}

	bAddr3, err := a.readData(card, offsetAddr3, lengthAddr3)
	if err == nil {
		data.Address3 = a.cleanString(bAddr3)
	}

	bPostcode, err := a.readData(card, offsetPostcode, lengthPostcode)
	if err == nil {
		data.Postcode = a.cleanString(bPostcode)
	}

	bCity, err := a.readData(card, offsetCity, lengthCity)
	if err == nil {
		data.City = a.cleanString(bCity)
	}

	bReligion, err := a.readData(card, offsetReligion, lengthReligion)
	if err == nil {
		data.Religion = a.cleanString(bReligion)
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

// ============================
// FRONTEND-EXPOSED FUNCTIONS
// ============================

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

	runtime.EventsEmit(a.ctx, "readCompleted", map[string]interface{}{
		"success":  true,
		"filePath": filePath,
	})

	return string(jsonData), nil
}

func (a *App) GetLatestData() (string, error) {
	appState.RLock()
	defer appState.RUnlock()

	if appState.CurrentData == nil {
		return `{"hasData": false, "error": "No data available"}`, nil
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

	runtime.EventsEmit(a.ctx, "autoReadStatusChanged", enabled)

	if enabled {
		log.Println("Auto-read feature enabled")
	} else {
		log.Println("Auto-read feature disabled")
	}
}

func (a *App) GetAutoReadStatus() bool {
	appState.RLock()
	defer appState.RUnlock()
	return appState.AutoRead
}

func (a *App) GetSavedDataFiles() ([]string, error) {
	const dataDir = "mykad_data"

	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	files, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read data directory: %w", err)
	}

	var fileList []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			fileList = append(fileList, file.Name())
		}
	}

	return fileList, nil
}

func (a *App) LoadDataFromFile(filename string) (string, error) {
	const dataDir = "mykad_data"
	filePath := filepath.Join(dataDir, filename)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	var kadData MyKadData
	if err := json.Unmarshal(data, &kadData); err != nil {
		return "", fmt.Errorf("invalid data file: %w", err)
	}

	appState.Lock()
	appState.CurrentData = &kadData
	appState.LastReadTime = time.Now()
	appState.Unlock()

	result := map[string]interface{}{
		"data":     kadData,
		"filePath": filePath,
		"success":  true,
	}

	jsonData, err := json.Marshal(result)
	if err != nil {
		return "", err
	}

	return string(jsonData), nil
}

// ============================
// AUTO-READ MONITOR
// ============================

func (a *App) startAutoReadMonitor() {
	var lastCardState bool = false

	log.Println("Auto-read monitor started")

	for {
		appState.RLock()
		autoReadEnabled := appState.AutoRead
		isReading := appState.IsReading
		appState.RUnlock()

		if !autoReadEnabled {
			time.Sleep(2 * time.Second)
			continue
		}

		status := a.CheckReaderStatus()

		if status.HasCard && !lastCardState && !isReading {
			log.Println("MyKad detected! Auto-reading data...")

			runtime.EventsEmit(a.ctx, "autoReadStarted", nil)

			data, filePath, err := a.ReadAndSaveCardData()
			if err != nil {
				log.Printf("Error auto-reading card: %v", err)
				runtime.EventsEmit(a.ctx, "autoReadError", err.Error())
			} else {
				appState.Lock()
				appState.CurrentData = data
				appState.LastReadTime = time.Now()
				appState.ReaderName = status.Reader
				appState.Unlock()

				log.Printf("✅ Auto-read successful! Saved to: %s", filePath)

				runtime.EventsEmit(a.ctx, "autoReadCompleted", map[string]interface{}{
					"data":     data,
					"filePath": filePath,
					"success":  true,
				})
			}
		}

		if !status.HasCard && lastCardState {
			log.Println("MyKad removed")
			appState.Lock()
			appState.CurrentData = nil
			appState.Unlock()

			runtime.EventsEmit(a.ctx, "cardRemoved", nil)
		}

		lastCardState = status.HasCard
		time.Sleep(1 * time.Second)
	}
}

// ============================
// APPLICATION EVENTS
// ============================

func (a *App) domReady(ctx context.Context) {
	log.Println("Frontend DOM is ready")
}

func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	log.Println("Application is closing...")

	appState.Lock()
	defer appState.Unlock()

	appState.CurrentData = nil
	appState.IsReading = false
	appState.AutoRead = false

	return false
}

func (a *App) shutdown(ctx context.Context) {
	log.Println("Application is shutting down")
}

// ============================
// UTILITY FUNCTIONS
// ============================

func (a *App) GetAppVersion() string {
	return "1.0.0"
}

func (a *App) GetAppInfo() map[string]interface{} {
	appState.RLock()
	defer appState.RUnlock()

	return map[string]interface{}{
		"version":   "1.0.0",
		"name":      "MyKad Reader Desktop",
		"autoRead":  appState.AutoRead,
		"hasData":   appState.CurrentData != nil,
		"lastRead":  appState.LastReadTime.Format("2006-01-02 15:04:05"),
		"isReading": appState.IsReading,
	}
}

func (a *App) OpenDataDirectory() error {
	const dataDir = "mykad_data"

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	runtime.BrowserOpenURL(a.ctx, "file://"+dataDir)

	return nil
}

// ============================
// MAIN FUNCTION
// ============================

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "MyKad Reader Desktop",
		Width:  1000,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 1},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnBeforeClose:    app.beforeClose,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		log.Fatal(err)
	}
}
