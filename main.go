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
