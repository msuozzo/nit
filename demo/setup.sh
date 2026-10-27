# Sourced, hidden, by demo.tape: builds nit and makes a small jj change to
# review, in /tmp/nit-demo with its own HOME so no real state shows.
demo=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
rm -rf /tmp/nit-demo && mkdir -p /tmp/nit-demo/home /tmp/nit-demo/repo
(cd "$demo/.." && go build -trimpath -o /tmp/nit-demo/bin/nit .) || return
export PATH="/tmp/nit-demo/bin:$PATH" HOME=/tmp/nit-demo/home
export JJ_USER=Demo JJ_EMAIL=demo@example.com NIT_SEND="$demo/agent.sh"
cd /tmp/nit-demo/repo && jj git init --quiet --no-colocate
printf 'module example.com/stats\n\ngo 1.22\n' > go.mod
cat > stats.go <<'GO'
package stats

// Sum adds up xs.
func Sum(xs []float64) float64 {
	var s float64
	for _, x := range xs {
		s += x
	}
	return s
}
GO
jj commit --quiet -m "Add Sum"
cat > stats.go <<'GO'
package stats

// Sum adds up xs.
func Sum(xs []float64) float64 {
	var total float64
	for _, x := range xs {
		total += x
	}
	return total
}

// Mean is the average of xs, or 0 when there are none.
func Mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	tmp := Sum(xs)
	return tmp / float64(len(xs))
}
GO
jj describe --quiet -m "Add Mean"
