package stats

import (
	"fmt"
	"strings"
)

const progressChar = "█"

func progressBar(percentComplete int, barSize int) string {
	p := float32(percentComplete) / 100.0
	repeatChars := int(float32(barSize) * p)
	return fmt.Sprintf(
		"|%s%s|  %d%%",
		strings.Repeat(progressChar, repeatChars),
		strings.Repeat(" ", barSize-repeatChars),
		percentComplete,
	)
}
