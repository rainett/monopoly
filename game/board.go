package game

type SpaceType string

const (
	SpaceProperty       SpaceType = "property"
	SpaceRailroad       SpaceType = "railroad"
	SpaceUtility        SpaceType = "utility"
	SpaceTax            SpaceType = "tax"
	SpaceChance         SpaceType = "chance"
	SpaceCommunityChest SpaceType = "community_chest"
	SpaceGo             SpaceType = "go"
	SpaceJail           SpaceType = "jail"
	SpaceFreeParking    SpaceType = "free_parking"
	SpaceGoToJail       SpaceType = "go_to_jail"
)

type ColorGroup string

const (
	ColorBrown     ColorGroup = "brown"
	ColorLightBlue ColorGroup = "light_blue"
	ColorPink      ColorGroup = "pink"
	ColorOrange    ColorGroup = "orange"
	ColorRed       ColorGroup = "red"
	ColorYellow    ColorGroup = "yellow"
	ColorGreen     ColorGroup = "green"
	ColorDarkBlue  ColorGroup = "dark_blue"
)

type BoardSpace struct {
	Position  int        `json:"position"`
	Name      string     `json:"name"`
	Type      SpaceType  `json:"type"`
	Color     ColorGroup `json:"color,omitempty"`
	Price     int        `json:"price,omitempty"`
	Rent      int        `json:"rent,omitempty"`
	TaxAmount int        `json:"taxAmount,omitempty"`
	GroupSize int        `json:"groupSize,omitempty"`
}

var Board = [40]BoardSpace{
	{0, "GO", SpaceGo, "", 0, 0, 0, 0},
	{1, "Mediterranean Ave", SpaceProperty, ColorBrown, 60, 2, 0, 2},
	{2, "Community Chest", SpaceCommunityChest, "", 0, 0, 0, 0},
	{3, "Baltic Ave", SpaceProperty, ColorBrown, 60, 4, 0, 2},
	{4, "Income Tax", SpaceTax, "", 0, 0, 200, 0},
	{5, "Reading Railroad", SpaceRailroad, "", 200, 25, 0, 0},
	{6, "Oriental Ave", SpaceProperty, ColorLightBlue, 100, 6, 0, 3},
	{7, "Chance", SpaceChance, "", 0, 0, 0, 0},
	{8, "Vermont Ave", SpaceProperty, ColorLightBlue, 100, 6, 0, 3},
	{9, "Connecticut Ave", SpaceProperty, ColorLightBlue, 120, 8, 0, 3},
	{10, "Jail", SpaceJail, "", 0, 0, 0, 0},
	{11, "St. Charles Place", SpaceProperty, ColorPink, 140, 10, 0, 3},
	{12, "Electric Company", SpaceUtility, "", 150, 0, 0, 0},
	{13, "States Ave", SpaceProperty, ColorPink, 140, 10, 0, 3},
	{14, "Virginia Ave", SpaceProperty, ColorPink, 160, 12, 0, 3},
	{15, "Pennsylvania Railroad", SpaceRailroad, "", 200, 25, 0, 0},
	{16, "St. James Place", SpaceProperty, ColorOrange, 180, 14, 0, 3},
	{17, "Community Chest", SpaceCommunityChest, "", 0, 0, 0, 0},
	{18, "Tennessee Ave", SpaceProperty, ColorOrange, 180, 14, 0, 3},
	{19, "New York Ave", SpaceProperty, ColorOrange, 200, 16, 0, 3},
	{20, "Free Parking", SpaceFreeParking, "", 0, 0, 0, 0},
	{21, "Kentucky Ave", SpaceProperty, ColorRed, 220, 18, 0, 3},
	{22, "Chance", SpaceChance, "", 0, 0, 0, 0},
	{23, "Indiana Ave", SpaceProperty, ColorRed, 220, 18, 0, 3},
	{24, "Illinois Ave", SpaceProperty, ColorRed, 240, 20, 0, 3},
	{25, "B&O Railroad", SpaceRailroad, "", 200, 25, 0, 0},
	{26, "Atlantic Ave", SpaceProperty, ColorYellow, 260, 22, 0, 3},
	{27, "Ventnor Ave", SpaceProperty, ColorYellow, 260, 22, 0, 3},
	{28, "Water Works", SpaceUtility, "", 150, 0, 0, 0},
	{29, "Marvin Gardens", SpaceProperty, ColorYellow, 280, 24, 0, 3},
	{30, "Go To Jail", SpaceGoToJail, "", 0, 0, 0, 0},
	{31, "Pacific Ave", SpaceProperty, ColorGreen, 300, 26, 0, 3},
	{32, "North Carolina Ave", SpaceProperty, ColorGreen, 300, 26, 0, 3},
	{33, "Community Chest", SpaceCommunityChest, "", 0, 0, 0, 0},
	{34, "Pennsylvania Ave", SpaceProperty, ColorGreen, 320, 28, 0, 3},
	{35, "Short Line", SpaceRailroad, "", 200, 25, 0, 0},
	{36, "Chance", SpaceChance, "", 0, 0, 0, 0},
	{37, "Park Place", SpaceProperty, ColorDarkBlue, 350, 35, 0, 2},
	{38, "Luxury Tax", SpaceTax, "", 0, 0, 100, 0},
	{39, "Boardwalk", SpaceProperty, ColorDarkBlue, 400, 50, 0, 2},
}

// CalculateRent returns the rent owed for landing on a space.
// ownerProperties is the list of positions owned by the property owner.
// diceTotal is needed for utility rent calculation.
func CalculateRent(space BoardSpace, ownerProperties []int, diceTotal int) int {
	switch space.Type {
	case SpaceProperty:
		// Check if owner has monopoly (all properties of same color group)
		colorCount := 0
		for _, pos := range ownerProperties {
			if pos >= 0 && pos < 40 && Board[pos].Color == space.Color {
				colorCount++
			}
		}
		if colorCount >= space.GroupSize {
			return space.Rent * 2 // Double rent with monopoly
		}
		return space.Rent

	case SpaceRailroad:
		rrCount := 0
		railroads := []int{5, 15, 25, 35}
		for _, pos := range ownerProperties {
			for _, rr := range railroads {
				if pos == rr {
					rrCount++
					break
				}
			}
		}
		switch rrCount {
		case 1:
			return 25
		case 2:
			return 50
		case 3:
			return 100
		case 4:
			return 200
		default:
			return 25
		}

	case SpaceUtility:
		utilCount := 0
		for _, pos := range ownerProperties {
			if pos == 12 || pos == 28 {
				utilCount++
			}
		}
		if utilCount >= 2 {
			return diceTotal * 10
		}
		return diceTotal * 4

	default:
		return 0
	}
}
