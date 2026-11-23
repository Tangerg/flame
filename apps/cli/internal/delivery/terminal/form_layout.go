package terminal

import "github.com/Tangerg/oolong/ui/core/layout"

const (
	compactDialogWidth  = 68
	standardDialogWidth = 88
	wideDialogWidth     = 96

	findDialogHeight     = 7
	standardDialogHeight = 18
	tallDialogHeight     = 24
)

const (
	minimumFormDialogHeight = 8
	formDialogFrameRows     = 2
	formDialogChromeColumns = 4
)

func formDialogPlacement(width, fieldCount, maximum int, body interface{ HeightForWidth(int) int }) layout.Placement {
	return layout.Placement{Width: width, Height: formDialogHeight(body.HeightForWidth(width-formDialogChromeColumns), fieldCount, maximum)}
}

func formDialogHeight(contentRows, fieldCount, maximum int) int {
	withValidation := layout.Sum(contentRows, fieldCount, formDialogFrameRows)
	return min(maximum, max(minimumFormDialogHeight, withValidation))
}
