package config

import "github.com/xuri/excelize/v2"

var ExcelStyles = map[string]excelize.Style{
	"border": {
		Border: []excelize.Border{
			{Type: "left", Style: 1, Color: "000000"},
			{Type: "right", Style: 1, Color: "000000"},
			{Type: "top", Style: 1, Color: "000000"},
			{Type: "bottom", Style: 1, Color: "000000"},
		},
	},
	"fill_gray": {
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"e5e7eb"},
			Pattern: 1,
		},
	},
	"fill_yellow": {
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"FFFF00"},
			Pattern: 1,
		},
	},
	"fill_green": {
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"#C6EFCE"}, // FFC6EFCE
			Pattern: 1,
		},
	},
	"fill_blue": {
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"#BDD7EE"}, // FFBDD7EE
			Pattern: 1,
		},
	},
	"header_border_bold": {
		Border: []excelize.Border{
			{Type: "left", Style: 1, Color: "000000"},
			{Type: "right", Style: 1, Color: "000000"},
			{Type: "top", Style: 1, Color: "000000"},
			{Type: "bottom", Style: 1, Color: "000000"},
		},
		Font: &excelize.Font{
			Bold: true,
		},
	},
	"font_bold": {
		Font: &excelize.Font{
			Bold: true,
		},
	},
	"font_bold_size14": {
		Font: &excelize.Font{
			Bold: true,
			Size: 14,
		},
	},
}