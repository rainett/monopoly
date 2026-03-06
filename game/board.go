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
	Position       int        `json:"position"`
	Name           string     `json:"name"`
	Type           SpaceType  `json:"type"`
	Color          ColorGroup `json:"color,omitempty"`
	Price          int        `json:"price,omitempty"`
	Rent           int        `json:"rent,omitempty"`
	TaxAmount      int        `json:"taxAmount,omitempty"`
	GroupSize      int        `json:"groupSize,omitempty"`
	HouseCost      int        `json:"houseCost,omitempty"`      // Cost to build one house
	RentWithHouses [6]int     `json:"rentWithHouses,omitempty"` // Rent with 0-5 houses (5 = hotel)
}

var Board = [40]BoardSpace{
	{Position: 0, Name: "GO", Type: SpaceGo},
	{Position: 1, Name: "Mediterranean Ave", Type: SpaceProperty, Color: ColorBrown, Price: 60, Rent: 2, GroupSize: 2, HouseCost: 50, RentWithHouses: [6]int{2, 10, 30, 90, 160, 250}},
	{Position: 2, Name: "Community Chest", Type: SpaceCommunityChest},
	{Position: 3, Name: "Baltic Ave", Type: SpaceProperty, Color: ColorBrown, Price: 60, Rent: 4, GroupSize: 2, HouseCost: 50, RentWithHouses: [6]int{4, 20, 60, 180, 320, 450}},
	{Position: 4, Name: "Income Tax", Type: SpaceTax, TaxAmount: 200},
	{Position: 5, Name: "Reading Railroad", Type: SpaceRailroad, Price: 200, Rent: 25},
	{Position: 6, Name: "Oriental Ave", Type: SpaceProperty, Color: ColorLightBlue, Price: 100, Rent: 6, GroupSize: 3, HouseCost: 50, RentWithHouses: [6]int{6, 30, 90, 270, 400, 550}},
	{Position: 7, Name: "Chance", Type: SpaceChance},
	{Position: 8, Name: "Vermont Ave", Type: SpaceProperty, Color: ColorLightBlue, Price: 100, Rent: 6, GroupSize: 3, HouseCost: 50, RentWithHouses: [6]int{6, 30, 90, 270, 400, 550}},
	{Position: 9, Name: "Connecticut Ave", Type: SpaceProperty, Color: ColorLightBlue, Price: 120, Rent: 8, GroupSize: 3, HouseCost: 50, RentWithHouses: [6]int{8, 40, 100, 300, 450, 600}},
	{Position: 10, Name: "Jail", Type: SpaceJail},
	{Position: 11, Name: "St. Charles Place", Type: SpaceProperty, Color: ColorPink, Price: 140, Rent: 10, GroupSize: 3, HouseCost: 100, RentWithHouses: [6]int{10, 50, 150, 450, 625, 750}},
	{Position: 12, Name: "Electric Company", Type: SpaceUtility, Price: 150},
	{Position: 13, Name: "States Ave", Type: SpaceProperty, Color: ColorPink, Price: 140, Rent: 10, GroupSize: 3, HouseCost: 100, RentWithHouses: [6]int{10, 50, 150, 450, 625, 750}},
	{Position: 14, Name: "Virginia Ave", Type: SpaceProperty, Color: ColorPink, Price: 160, Rent: 12, GroupSize: 3, HouseCost: 100, RentWithHouses: [6]int{12, 60, 180, 500, 700, 900}},
	{Position: 15, Name: "Pennsylvania Railroad", Type: SpaceRailroad, Price: 200, Rent: 25},
	{Position: 16, Name: "St. James Place", Type: SpaceProperty, Color: ColorOrange, Price: 180, Rent: 14, GroupSize: 3, HouseCost: 100, RentWithHouses: [6]int{14, 70, 200, 550, 750, 950}},
	{Position: 17, Name: "Community Chest", Type: SpaceCommunityChest},
	{Position: 18, Name: "Tennessee Ave", Type: SpaceProperty, Color: ColorOrange, Price: 180, Rent: 14, GroupSize: 3, HouseCost: 100, RentWithHouses: [6]int{14, 70, 200, 550, 750, 950}},
	{Position: 19, Name: "New York Ave", Type: SpaceProperty, Color: ColorOrange, Price: 200, Rent: 16, GroupSize: 3, HouseCost: 100, RentWithHouses: [6]int{16, 80, 220, 600, 800, 1000}},
	{Position: 20, Name: "Free Parking", Type: SpaceFreeParking},
	{Position: 21, Name: "Kentucky Ave", Type: SpaceProperty, Color: ColorRed, Price: 220, Rent: 18, GroupSize: 3, HouseCost: 150, RentWithHouses: [6]int{18, 90, 250, 700, 875, 1050}},
	{Position: 22, Name: "Chance", Type: SpaceChance},
	{Position: 23, Name: "Indiana Ave", Type: SpaceProperty, Color: ColorRed, Price: 220, Rent: 18, GroupSize: 3, HouseCost: 150, RentWithHouses: [6]int{18, 90, 250, 700, 875, 1050}},
	{Position: 24, Name: "Illinois Ave", Type: SpaceProperty, Color: ColorRed, Price: 240, Rent: 20, GroupSize: 3, HouseCost: 150, RentWithHouses: [6]int{20, 100, 300, 750, 925, 1100}},
	{Position: 25, Name: "B&O Railroad", Type: SpaceRailroad, Price: 200, Rent: 25},
	{Position: 26, Name: "Atlantic Ave", Type: SpaceProperty, Color: ColorYellow, Price: 260, Rent: 22, GroupSize: 3, HouseCost: 150, RentWithHouses: [6]int{22, 110, 330, 800, 975, 1150}},
	{Position: 27, Name: "Ventnor Ave", Type: SpaceProperty, Color: ColorYellow, Price: 260, Rent: 22, GroupSize: 3, HouseCost: 150, RentWithHouses: [6]int{22, 110, 330, 800, 975, 1150}},
	{Position: 28, Name: "Water Works", Type: SpaceUtility, Price: 150},
	{Position: 29, Name: "Marvin Gardens", Type: SpaceProperty, Color: ColorYellow, Price: 280, Rent: 24, GroupSize: 3, HouseCost: 150, RentWithHouses: [6]int{24, 120, 360, 850, 1025, 1200}},
	{Position: 30, Name: "Go To Jail", Type: SpaceGoToJail},
	{Position: 31, Name: "Pacific Ave", Type: SpaceProperty, Color: ColorGreen, Price: 300, Rent: 26, GroupSize: 3, HouseCost: 200, RentWithHouses: [6]int{26, 130, 390, 900, 1100, 1275}},
	{Position: 32, Name: "North Carolina Ave", Type: SpaceProperty, Color: ColorGreen, Price: 300, Rent: 26, GroupSize: 3, HouseCost: 200, RentWithHouses: [6]int{26, 130, 390, 900, 1100, 1275}},
	{Position: 33, Name: "Community Chest", Type: SpaceCommunityChest},
	{Position: 34, Name: "Pennsylvania Ave", Type: SpaceProperty, Color: ColorGreen, Price: 320, Rent: 28, GroupSize: 3, HouseCost: 200, RentWithHouses: [6]int{28, 150, 450, 1000, 1200, 1400}},
	{Position: 35, Name: "Short Line", Type: SpaceRailroad, Price: 200, Rent: 25},
	{Position: 36, Name: "Chance", Type: SpaceChance},
	{Position: 37, Name: "Park Place", Type: SpaceProperty, Color: ColorDarkBlue, Price: 350, Rent: 35, GroupSize: 2, HouseCost: 200, RentWithHouses: [6]int{35, 175, 500, 1100, 1300, 1500}},
	{Position: 38, Name: "Luxury Tax", Type: SpaceTax, TaxAmount: 100},
	{Position: 39, Name: "Boardwalk", Type: SpaceProperty, Color: ColorDarkBlue, Price: 400, Rent: 50, GroupSize: 2, HouseCost: 200, RentWithHouses: [6]int{50, 200, 600, 1400, 1700, 2000}},
}

// CalculateRent returns the rent owed for landing on a space.
// ownerProperties is the list of positions owned by the property owner.
// diceTotal is needed for utility rent calculation.
// improvements is the number of houses (0-4) or hotel (5) on this property.
func CalculateRent(space BoardSpace, ownerProperties []int, diceTotal int, improvements int) int {
	switch space.Type {
	case SpaceProperty:
		// If there are improvements, use the improvement rent
		if improvements > 0 && improvements <= 5 {
			return space.RentWithHouses[improvements]
		}

		// Check if owner has monopoly (all properties of same color group)
		colorCount := 0
		for _, pos := range ownerProperties {
			if pos >= 0 && pos < 40 && Board[pos].Color == space.Color {
				colorCount++
			}
		}
		if colorCount >= space.GroupSize {
			return space.Rent * 2 // Double rent with monopoly (no houses)
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
