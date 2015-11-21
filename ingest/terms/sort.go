package terms

import (
    "sort"
)

// Term class to store a term and the occurance count
type Term struct {
    Text string
    Count uint64
}

// Terms sorting interface
type Terms []Term
func (terms Terms) Len() int {
    return len( terms )
}
func (terms Terms) Less(i, j int) bool {
    return terms[i].Count < terms[j].Count
}
func (terms Terms) Swap(i, j int) {
    terms[i], terms[j] = terms[j], terms[i]
}

// GetTermCounts returns a desc sorted array of term structs of size N.
func GetTermCounts( num uint64 ) []Term {
    terms := make( Terms, len( termCounts ) )
    i := 0
    for term, count := range termCounts {
        terms[i] = Term{
            Text: term,
            Count: count,
        }
        i++
    }
    sort.Sort( sort.Reverse( terms ) )
    numTerms := uint64( len( terms ) )
    if num > numTerms {
        return terms[0:numTerms]
    }
    return terms[0:num]
}

// GetTopTermsSlice returns a desc sorted array of N top terms.
func GetTopTermsSlice( num uint64 ) []string {
    terms := GetTermCounts( num )
    topTerms := make( []string, len( termCounts ) )
    for i, term := range terms {
        topTerms[i] = term.Text
    }
    return topTerms[0:num]
}

// GetTopTermsMap returns a map of N top terms, terms as keys, true as values.
func GetTopTermsMap( num uint64 ) map[string]bool {
    terms := GetTermCounts( num )
    topTerms := make( map[string]bool )
    for _, term := range terms {
        topTerms[term.Text] = true
    }
    return topTerms
}