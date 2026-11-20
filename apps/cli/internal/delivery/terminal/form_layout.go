package terminal

import "github.com/Tangerg/oolong/ui/core/layout"

// Dialog widths form one scale so dialogs of the same weight line up; each
// dialog's height follows its own content.
const (
	compactDialogWidth  = 68
	standardDialogWidth = 88
	wideDialogWidth     = 96
)

const (
	minimumFormDialogHeight = 8
	formDialogFrameRows     = 2
	// formDialogChromeColumns is the kit dialog's border plus its horizontal
	// padding, which narrow the body below the placement width.
	formDialogChromeColumns = 4
)

// formDialogPlacement sizes a form dialog to its body measured at the width
// the body actually receives inside the dialog chrome.
func formDialogPlacement(width, fieldCount, maximum int, body interface{ HeightForWidth(int) int }) layout.Placement {
	return layout.Placement{Width: width, Height: formDialogHeight(body.HeightForWidth(width-formDialogChromeColumns), fieldCount, maximum)}
}

func formDialogHeight(contentRows, fieldCount, maximum int) int {
	withValidation := layout.Sum(contentRows, fieldCount, formDialogFrameRows)
	return min(maximum, max(minimumFormDialogHeight, withValidation))
}
