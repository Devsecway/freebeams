//go:build darwin

package spell

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>
#include <stdlib.h>
#include <string.h>

// fw_spell_orthography is the orthography for lang ("" or NULL: the
// checker's own language).
static NSOrthography *fw_spell_orthography(NSSpellChecker *c, const char *lang) {
	NSString *l = (lang != NULL && lang[0] != 0) ? [NSString stringWithUTF8String:lang] : c.language;
	if (l == nil || l.length == 0) {
		l = @"en";
	}
	return [NSOrthography defaultOrthographyForLanguage:l];
}

// fw_spell_correct checks n words joined by '\n' (no word contains one) in
// language lang. out[i] is set to 1 for each misspelled word i. It runs the
// unified text checker in the background (requestCheckingOfString, whose
// completion handler runs on a queue of the checker's), so it may be called
// from any goroutine; it waits at most two seconds. It returns 0, or -1 on a
// timeout (out is then untouched).
static int fw_spell_correct(const char *joined, const char *lang, unsigned char *out, int n) {
	@autoreleasepool {
		NSString *s = [NSString stringWithUTF8String:joined];
		if (s == nil) {
			return -1;
		}
		NSSpellChecker *c = [NSSpellChecker sharedSpellChecker];
		// UTF-16 offsets of each word's start.
		NSMutableArray<NSNumber *> *starts = [NSMutableArray arrayWithCapacity:n];
		[starts addObject:@0];
		for (NSUInteger i = 0; i < s.length; i++) {
			if ([s characterAtIndex:i] == '\n') {
				[starts addObject:@(i + 1)];
			}
		}
		dispatch_semaphore_t done = dispatch_semaphore_create(0);
		__block NSArray<NSTextCheckingResult *> *found = nil;
		NSDictionary *opts = @{NSTextCheckingOrthographyKey: fw_spell_orthography(c, lang)};
		[c requestCheckingOfString:s range:NSMakeRange(0, s.length) types:NSTextCheckingTypeSpelling options:opts
			inSpellDocumentWithTag:0 completionHandler:^(NSInteger seq, NSArray<NSTextCheckingResult *> *results,
				NSOrthography *orth, NSInteger words) {
			found = results;
			dispatch_semaphore_signal(done);
		}];
		if (dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, 2 * NSEC_PER_SEC)) != 0) {
			return -1;
		}
		for (NSTextCheckingResult *r in found) {
			if (r.resultType != NSTextCheckingTypeSpelling) {
				continue;
			}
			// The word whose start is the last one at or before the range.
			NSUInteger lo = 0, hi = starts.count;
			while (hi - lo > 1) {
				NSUInteger mid = (lo + hi) / 2;
				if (starts[mid].unsignedIntegerValue <= r.range.location) {
					lo = mid;
				} else {
					hi = mid;
				}
			}
			if ((int)lo < n) {
				out[lo] = 1;
			}
		}
		return 0;
	}
}

// fw_spell_guesses returns the checker's guesses for word, joined by '\n'
// (malloc'd; the caller frees it), or NULL.
static char *fw_spell_guesses(const char *word, const char *lang) {
	@autoreleasepool {
		NSString *w = [NSString stringWithUTF8String:word];
		if (w == nil) {
			return NULL;
		}
		NSSpellChecker *c = [NSSpellChecker sharedSpellChecker];
		NSString *l = (lang != NULL && lang[0] != 0) ? [NSString stringWithUTF8String:lang] : nil;
		NSArray<NSString *> *g = [c guessesForWordRange:NSMakeRange(0, w.length) inString:w language:l inSpellDocumentWithTag:0];
		if (g.count == 0) {
			return NULL;
		}
		return strdup([[g componentsJoinedByString:@"\n"] UTF8String]);
	}
}

// fw_spell_languages returns the checker's languages, its current one first,
// joined by '\n' (malloc'd).
static char *fw_spell_languages(void) {
	@autoreleasepool {
		NSSpellChecker *c = [NSSpellChecker sharedSpellChecker];
		NSMutableArray<NSString *> *all = [NSMutableArray array];
		if (c.language.length > 0) {
			[all addObject:c.language];
		}
		for (NSString *l in c.availableLanguages) {
			if (![all containsObject:l]) {
				[all addObject:l];
			}
		}
		return strdup([[all componentsJoinedByString:@"\n"] UTF8String]);
	}
}
*/
import "C"

import (
	"log"
	"strings"
	"sync"
	"unsafe"
)

// nsSpell is the macOS system spelling checker (NSSpellChecker): the user's
// languages and learned words, the same verdicts as every Mac text field.
// The personal dictionary stays the Engine's (the app's settings): Learn never
// writes the user's system-wide dictionary.
type nsSpell struct {
	mu    sync.Mutex // one request at a time
	langs []string
	once  sync.Once
}

var systemChecker nsSpell

// System returns the macOS system checker.
func System() Backend { return &systemChecker }

func (s *nsSpell) Name() string { return "macOS" }

func (s *nsSpell) Languages() []string {
	s.once.Do(func() {
		p := C.fw_spell_languages()
		if p != nil {
			s.langs = strings.Split(C.GoString(p), "\n")
			C.free(unsafe.Pointer(p))
		}
	})
	return s.langs
}

// Correct checks the words in one background request (Backend). A word that
// contains a newline is reported correct.
func (s *nsSpell) Correct(words []string, lang string) []bool {
	out := make([]bool, len(words))
	for i := range out {
		out[i] = true
	}
	if len(words) == 0 {
		return out
	}
	clean := make([]string, len(words))
	for i, w := range words {
		clean[i] = strings.ReplaceAll(w, "\n", " ")
	}
	cs := C.CString(strings.Join(clean, "\n"))
	defer C.free(unsafe.Pointer(cs))
	cl := C.CString(lang)
	defer C.free(unsafe.Pointer(cl))
	flags := make([]byte, len(words))
	s.mu.Lock()
	rc := C.fw_spell_correct(cs, cl, (*C.uchar)(unsafe.Pointer(&flags[0])), C.int(len(words)))
	s.mu.Unlock()
	if rc != 0 {
		log.Printf("[spell] the system spelling checker did not answer; %d word(s) left unchecked", len(words))
		return out
	}
	for i, f := range flags {
		out[i] = f == 0
	}
	return out
}

// Suggest returns the system checker's guesses (Backend).
func (s *nsSpell) Suggest(word, lang string) []string {
	cw := C.CString(word)
	defer C.free(unsafe.Pointer(cw))
	cl := C.CString(lang)
	defer C.free(unsafe.Pointer(cl))
	p := C.fw_spell_guesses(cw, cl)
	if p == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(p))
	return strings.Split(C.GoString(p), "\n")
}
