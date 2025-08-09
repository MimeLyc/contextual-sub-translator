package media

// TVShowInfo represents TV show information parsed from tvshow.nfo file
type TVShowInfo struct {
	Title         string   `xml:"title"`         // show title
	OriginalTitle string   `xml:"originaltitle"` // original title
	Plot          string   `xml:"plot"`          // plot summary
	Genre         []string `xml:"genre"`         // genre tags
	Premiered     string   `xml:"premiered"`     // premiere date
	Rating        float32  `xml:"rating"`        // rating
	Studio        string   `xml:"studio"`        // production studio
	Actors        []Actor  `xml:"actor"`         // cast list
	Aired         string   `xml:"aired"`         // air date
	Year          int      `xml:"year"`          // year
	Season        int      `xml:"season"`        // current season
}

// Actor represents actor information
type Actor struct {
	Name  string `xml:"name"`
	Role  string `xml:"role"`
	Order int    `xml:"order"`
}
