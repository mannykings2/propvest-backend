package validators

import (
	"regexp"

	"github.com/mannykings2/propvest-backend/internal/errors"
)

// ValidateNigerianPhone enforces E.164 phone format for Nigerian numbers
//
// E.164 format: +[country code][number]
// Nigerian numbers: +234[area code][number]
//
// Valid examples:
//   +2348012345678 (MTN)
//   +2347012345678 (MTN)
//   +2349012345678 (MTN)
//   +2348112345678 (Airtel)
//   +2349087654321 (Etisalat/9mobile)
//
// Invalid examples:
//   08012345678 (missing country code)
//   2348012345678 (missing +)
//   +234 801 234 5678 (spaces not allowed)
//   +234-801-234-5678 (hyphens not allowed)
//
// Why E.164?
//   - International standard for phone numbers
//   - Works with SMS providers worldwide (Twilio, Termii, etc.)
//   - Consistent format in database
//   - Enables phone number portability
//   - Required for WhatsApp Business API
//
// Parameters:
//   - phone: Phone number string to validate
//
// Returns:
//   - nil if phone matches E.164 Nigerian format
//   - ErrInvalidPhoneFormat if format is invalid
//
// Example usage:
//   if err := validators.ValidateNigerianPhone(phone); err != nil {
//       return err
//   }
func ValidateNigerianPhone(phone string) error {
	// Regex breakdown:
	//   ^\+234       - Must start with +234 (Nigerian country code)
	//   [789]        - Area code first digit: 7, 8, or 9 (Nigerian mobile networks)
	//                  7xx: MTN, Glo
	//                  8xx: MTN, Airtel, Glo, 9mobile
	//                  9xx: MTN, Airtel, 9mobile
	//   [01]         - Area code second digit: 0 or 1
	//   \d{8}$       - Exactly 8 more digits (total 10 digits after +234)
	//
	// Total format: +234 + [789] + [01] + 8 digits = 14 characters
	// Example: +2348012345678
	//          ^^^^ ^^  ^^^^^^^^
	//          |    |   |
	//          |    |   └─ 8 digits
	//          |    └───── Area code (70, 80, 81, 90, 91)
	//          └────────── Country code (Nigeria)
	matched, _ := regexp.MatchString(`^\+234[789][01]\d{8}$`, phone)
	if !matched {
		return errors.ErrInvalidPhoneFormat
	}

	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// TEACHING NOTES: Phone Number Validation
// ═══════════════════════════════════════════════════════════════════════════
//
// E.164 Format Structure:
//   [+] [country code] [subscriber number]
//   Example: +234 8012345678
//
// Nigerian Mobile Network Codes:
//   - 703, 706, 803, 806, 810, 813, 814, 816, 903, 906 (MTN)
//   - 701, 708, 802, 808, 812, 901, 902, 904, 907, 912 (Airtel)
//   - 705, 805, 807, 811, 815, 905 (Glo)
//   - 809, 817, 818, 909, 908 (9mobile/Etisalat)
//
// Why Validate Phone Format?
//   1. Data Quality: Ensures phone numbers are usable
//   2. SMS Delivery: Invalid format = failed SMS delivery
//   3. Cost Savings: Don't pay for failed SMS attempts
//   4. User Experience: Early error feedback
//   5. Security: Prevents injection attacks
//
// Why E.164 Specifically?
//   - SMS providers require it (Twilio, Africa's Talking, Termii)
//   - WhatsApp API requires it
//   - Google Firebase requires it
//   - International portability
//
// Alternative Approaches:
//
// 1. Library-based (google/libphonenumber):
//    import "github.com/nyaruka/phonenumbers"
//    num, err := phonenumbers.Parse(phone, "NG")
//    Pros: Handles all countries, very robust
//    Cons: Heavy dependency (2MB+), overkill for Nigeria-only
//
// 2. Regex (current approach):
//    Pros: Lightweight, fast, simple
//    Cons: Nigeria-only, must update if telecom adds new prefixes
//
// 3. API-based (Numverify, Abstract API):
//    Pros: Always up-to-date, validates real numbers
//    Cons: Cost per validation, latency, external dependency
//
// When to Expand:
//   - If supporting other African countries: use libphonenumber
//   - If needing carrier lookup: use Numverify API
//   - If checking if number exists: use HLR lookup service
//
// Future Enhancement Ideas:
//   - Validate that phone number actually exists (HLR lookup)
//   - Check if number is mobile vs landline
//   - Identify carrier (MTN, Airtel, etc.)
//   - Detect VoIP/virtual numbers
//   - Check if number is on DND (Do Not Disturb) list
//
// ═══════════════════════════════════════════════════════════════════════════
