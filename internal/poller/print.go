package poller

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/atvirokodosprendimai/go-modpoll/internal/domain"
)

// PrintTable writes a fixed-format table of references for a device using a
// stdlib tabwriter. Floats are rounded to the given precision.
func PrintTable(dev *domain.Device, precision int) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()
	fmt.Fprintln(w, "Reference\tValue\tUnit")
	names := make([]string, 0, len(dev.References))
	for n := range dev.References {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		ref := dev.References[n]
		val := roundIfFloat(ref.Val, precision)
		fmt.Fprintf(w, "%s\t%v\t%s\n", ref.Name, val, ref.Unit)
	}
}
