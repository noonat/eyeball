package argumentwrapping

import "fmt"

// Report writes a half-wrapped call.
func Report(b *fmt.Stringer, n, m int) {
	fmt.Printf("round %d, %d files\n",
		n, m)
}
