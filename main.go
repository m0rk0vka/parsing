package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/net/html/charset"
)

type ValCurs struct {
	XMLName xml.Name `xml:"ValCurs"`
	Text    string   `xml:",chardata"`
	Date    string   `xml:"Date,attr"`
	Name    string   `xml:"name,attr"`
	Valute  []struct {
		Text      string `xml:",chardata"`
		ID        string `xml:"ID,attr"`
		NumCode   string `xml:"NumCode"`
		CharCode  string `xml:"CharCode"`
		Nominal   string `xml:"Nominal"`
		Name      string `xml:"Name"`
		Value     string `xml:"Value"`
		VunitRate string `xml:"VunitRate"`
	} `xml:"Valute"`
}

type ValueWithDate struct {
	Value float64
	Date  string
}

type ValuteWithValueWithDate struct {
	CharCode string
	Value    float64
	Date     string
}

var ValuteDict = make(map[string][]ValueWithDate)
var lock = sync.RWMutex{}

const (
	DDMMYYYY        = "02/01/2006"
	url      string = "https://www.cbr.ru/scripts/XML_daily_eng.asp?date_req="
)

func GetDayStruct(date string) (ValCurs, error) {
	req, err := http.NewRequest("GET", url+date, nil)
	if err != nil {
		return ValCurs{}, fmt.Errorf("NewRequest: %v", err)
	}

	req.Header.Set("User-Agent", "test")
	req.Header.Set("Accept", "*/*")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return ValCurs{}, fmt.Errorf("Do request: %v", err)
	}

	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("ReadAll: %v", err)
	}

	var vc ValCurs

	//fmt.Println(string(data))

	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.CharsetReader = charset.NewReaderLabel

	if err := decoder.Decode(&vc); err != nil {
		return ValCurs{}, fmt.Errorf("Decode: %v", err)
	}

	return vc, nil
}

func CheckString(value string) string {
	for i, b := range []byte(value) {
		if b == []byte(",")[0] {
			return value[:i] + "." + value[i+1:]
		}
	}

	return value
}

func create(key string) {
	lock.Lock()
	defer lock.Unlock()

	_, ok := ValuteDict[key]
	if !ok {
		ValuteDict[key] = []ValueWithDate{}
	}
}

func write(key string, val float64, date string) {
	lock.Lock()
	defer lock.Unlock()

	ValuteDict[key] = append(ValuteDict[key], ValueWithDate{
		Value: val,
		Date:  date,
	})
}

func GetDataFromStruct(vc ValCurs) error {
	for _, v := range vc.Valute {
		create(v.CharCode)
		floatValue, err := strconv.ParseFloat(CheckString(v.Value), 64)
		if err != nil {
			return fmt.Errorf("strconv.ParseFloat: %v", err)
		}
		write(v.CharCode, floatValue, vc.Date)
	}

	return nil
}

func GetValues() (ValuteWithValueWithDate, ValuteWithValueWithDate, float64) {
	minimum := ValuteWithValueWithDate{
		Value: float64(1000000),
		Date:  DDMMYYYY,
	}
	maximum := ValuteWithValueWithDate{
		Value: float64(0),
		Date:  DDMMYYYY,
	}
	count := 0
	sum := float64(0)

	for key, valueList := range ValuteDict {
		for _, value := range valueList {
			count += 1
			sum += value.Value
			if value.Value < minimum.Value {
				minimum.CharCode = key
				minimum.Value = value.Value
				minimum.Date = value.Date
			}
			if value.Value > maximum.Value {
				maximum.CharCode = key
				maximum.Value = value.Value
				maximum.Date = value.Date
			}
		}
	}

	return maximum, minimum, (sum / float64(count))
}

func GetDataForLast90Days() error {
	now := time.Now().UTC()

	var wg sync.WaitGroup

	for i := 0; i < 90; i++ {
		wg.Add(1)
		now = now.AddDate(0, 0, -1)
		dateInFormat := now.Format(DDMMYYYY)

		go func(dateInFormat string) {
			defer wg.Done()
			vc, err := GetDayStruct(dateInFormat)
			if err != nil {
				log.Fatalf("GetDayStruct: %v", err)
			}

			if err := GetDataFromStruct(vc); err != nil {
				log.Fatalf("GetDataFromStruct: %v", err)
			}
		}(dateInFormat)

	}

	wg.Wait()

	return nil
}

func main() {
	_ = GetDataForLast90Days()

	mx, mn, avg := GetValues()

	fmt.Printf("Максимальный курс для валюты %v составил %.2f рубля на %v\n", mx.CharCode, mx.Value, mx.Date)
	fmt.Printf("Минимальный курс для валюты %v составил %.2f рубля на %v\n", mn.CharCode, mn.Value, mn.Date)
	fmt.Printf("Среднее значение курса рубля составило: %.2f\n", avg)
}
