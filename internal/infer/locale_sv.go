package infer

import (
	"fmt"

	"github.com/brianvoe/gofakeit/v7"
)

var (
	svLastNames = []string{
		"Andersson", "Johansson", "Karlsson", "Nilsson", "Eriksson", "Larsson", "Olsson", "Persson", "Svensson", "Gustafsson",
		"Pettersson", "Jonsson", "Jansson", "Hansson", "Bengtsson", "Jönsson", "Lindberg", "Jakobsson", "Magnusson", "Olofsson",
		"Lindström", "Lindqvist", "Lindgren", "Berg", "Axelsson", "Bergström", "Lundberg", "Lind", "Lundgren", "Lundqvist",
		"Mattsson", "Berglund", "Fredriksson", "Sandberg", "Henriksson", "Forsberg", "Sjöberg", "Wallin", "Engström", "Eklund",
		"Danielsson", "Håkansson", "Lundin", "Gunnarsson", "Björk", "Bergman", "Holm", "Wikström", "Samuelsson", "Isaksson",
		"Fransson", "Bergqvist", "Nyström", "Holmberg", "Arvidsson", "Löfgren", "Söderberg", "Nyberg", "Blomqvist", "Claesson",
		"Mårtensson", "Nordström", "Lundström", "Viklund", "Björklund", "Eliasson", "Pålsson", "Berggren", "Sandström", "Lund",
		"Nordin", "Ström", "Åberg", "Ekström", "Hermansson", "Holmgren", "Falk", "Dahlberg", "Strömberg", "Sundberg",
		"Hellström", "Sjögren", "Ek", "Blom", "Abrahamsson", "Martinsson", "Öberg", "Andreasson", "Hansen", "Jonasson",
		"Norberg", "Åkesson", "Lindholm", "Holmqvist", "Sundström", "Ali", "Hedlund", "Sjöström", "Dahl", "Lindkvist",
	}
	svFirstNames = []string{
		"Lucas", "William", "Liam", "Oliver", "Hugo", "Elias", "Oscar", "Alexander", "Adam", "Noah",
		"Vincent", "Leo", "Ludvig", "Axel", "Filip", "Anton", "Theodor", "Erik", "Charlie", "Albin",
		"Alice", "Maja", "Elsa", "Astrid", "Wilma", "Freja", "Olivia", "Ebba", "Alma", "Alva",
		"Lilly", "Vera", "Klara", "Saga", "Selma", "Ella", "Agnes", "Stella", "Signe", "Nova",
		"Lars", "Mikael", "Anders", "Johan", "Per", "Karl", "Nils", "Jan", "Peter", "Thomas",
		"Daniel", "Fredrik", "Henrik", "Mats", "Stefan", "Magnus", "Andreas", "Martin", "Jonas", "Marcus",
		"Maria", "Anna", "Margareta", "Elisabeth", "Eva", "Birgitta", "Kristina", "Karin", "Marie", "Ingrid",
		"Sofia", "Linnéa", "Helena", "Emma", "Sara", "Linda", "Susanne", "Lena", "Malin", "Elin",
	}
	svCounties = []string{
		"Stockholms län", "Uppsala län", "Södermanlands län", "Östergötlands län", "Jönköpings län",
		"Kronobergs län", "Kalmar län", "Gotlands län", "Blekinge län", "Skåne län",
		"Hallands län", "Västra Götalands län", "Värmlands län", "Örebro län", "Västmanlands län",
		"Dalarnas län", "Gävleborgs län", "Västernorrlands län", "Jämtlands län", "Västerbottens län",
		"Norrbottens län",
	}
	svCities = []string{
		"Stockholm", "Göteborg", "Malmö", "Uppsala", "Västerås", "Örebro", "Linköping", "Helsingborg", "Jönköping", "Norrköping",
		"Lund", "Umeå", "Gävle", "Borås", "Södertälje", "Eskilstuna", "Halmstad", "Växjö", "Karlstad", "Sundsvall",
		"Östersund", "Trollhättan", "Luleå", "Kalmar", "Kristianstad", "Falun", "Skellefteå", "Karlskrona", "Skövde", "Uddevalla",
		"Visby", "Härnösand", "Mariestad", "Nyköping", "Motala", "Landskrona", "Örnsköldsvik", "Lidköping", "Piteå", "Sandviken",
	}
	svPhonePrefixes = []string{"070", "072", "073", "076", "079"}
	svStreetNames   = []string{
		"Storgatan", "Kungsgatan", "Drottninggatan", "Vasagatan", "Sveavägen",
		"Götgatan", "Hamngatan", "Skolgatan", "Kyrkogatan", "Nygatan",
		"Bergsgatan", "Norra vägen", "Södra vägen", "Östra gatan", "Västra gatan",
		"Parkvägen", "Linnégatan", "Odengatan", "Birger Jarlsgatan", "Karlavägen",
	}
)

func lastNameSV(f *gofakeit.Faker) any  { return randomFrom(f, svLastNames) }
func firstNameSV(f *gofakeit.Faker) any { return randomFrom(f, svFirstNames) }

func fullNameSV(f *gofakeit.Faker) any {
	return fmt.Sprintf("%s %s", randomFrom(f, svFirstNames), randomFrom(f, svLastNames))
}

func countySV(f *gofakeit.Faker) any { return randomFrom(f, svCounties) }
func citySV(f *gofakeit.Faker) any   { return randomFrom(f, svCities) }
func countrySV(*gofakeit.Faker) any  { return "Sverige" }

// addressSV returns "{street} {number}, {zip} {city}" (e.g.,
// "Storgatan 12, 111 22 Stockholm"). Street names use a small set of common
// Swedish suffixes to keep output recognizable without bundling a large
// dictionary.
func addressSV(f *gofakeit.Faker) any {
	number := f.Number(1, 200)
	zip := fmt.Sprintf("%03d %02d", f.Number(100, 999), f.Number(0, 99))
	return fmt.Sprintf("%s %d, %s %s", randomFrom(f, svStreetNames), number, zip, randomFrom(f, svCities))
}

// zipSV returns a Swedish postal code in "NNN NN" form (five digits with a
// space after the third), matching Posten's official format.
func zipSV(f *gofakeit.Faker) any {
	return fmt.Sprintf("%03d %02d", f.Number(100, 999), f.Number(0, 99))
}

// phoneSV returns a mobile-style number ("07X-NNN NN NN"). Swedish mobile
// numbers all start with 07 followed by an operator-assigned digit; fixed-line
// area codes vary in length, so we stick to mobile for predictability.
func phoneSV(f *gofakeit.Faker) any {
	return fmt.Sprintf("%s-%03d %02d %02d",
		randomFrom(f, svPhonePrefixes),
		f.Number(0, 999),
		f.Number(0, 99),
		f.Number(0, 99),
	)
}
