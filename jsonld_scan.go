package metagrab

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"

	"golang.org/x/net/html"
)

// scanJSONLD finds <script type="application/ld+json"> elements in the rest
// of a document (the body) without tokenizing every element. It streams
// from one '<' to the next, skips comments and the contents of other
// scripts, and parses only <script> start tags. This keeps RichFields close
// to the cost of reading the bytes, instead of a full HTML tokenization.
func scanJSONLD(r io.Reader, e *extractor) error {
	br := bufio.NewReaderSize(r, 32*1024)
	for e.wantJSONLD() {
		if err := skipPast(br, '<'); err != nil {
			return eofIsNil(err)
		}
		head, _ := br.Peek(7)
		switch {
		case bytes.HasPrefix(head, []byte("!--")):
			_, _ = br.Discard(3)
			if err := skipPastSeq(br, "-->"); err != nil {
				return eofIsNil(err)
			}
		case len(head) == 7 && strings.EqualFold(string(head[:6]), "script") && isTagNameEnd(head[6]):
			tag, err := readTag(br)
			if err != nil {
				return eofIsNil(err)
			}
			isLD := isLDJSONType(scriptType(tag))
			content, err := readUntilFold(br, "</script", isLD)
			if isLD {
				e.jsonLD(string(content))
			}
			if err != nil {
				return eofIsNil(err)
			}
		}
	}
	return nil
}

func eofIsNil(err error) error {
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

// skipPast discards input up to and including the next delim.
func skipPast(br *bufio.Reader, delim byte) error {
	for {
		_, err := br.ReadSlice(delim)
		if err == nil {
			return nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return err
		}
	}
}

// skipPastSeq discards input up to and including the next occurrence of seq.
func skipPastSeq(br *bufio.Reader, seq string) error {
	for {
		if err := skipPast(br, seq[0]); err != nil {
			return err
		}
		next, _ := br.Peek(len(seq) - 1)
		if string(next) == seq[1:] {
			_, err := br.Discard(len(seq) - 1)
			return err
		}
	}
}

// readTag reads the rest of a start tag (after '<') up to and including
// the closing '>', honouring quoted attribute values. It returns the tag
// with its leading '<' restored.
func readTag(br *bufio.Reader) ([]byte, error) {
	tag := []byte{'<'}
	var quote byte
	for {
		c, err := br.ReadByte()
		if err != nil {
			return nil, err
		}
		tag = append(tag, c)
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return tag, nil
		}
		if len(tag) > 16*1024 {
			return nil, errors.New("script tag too long")
		}
	}
}

// readUntilFold consumes input up to and including the next
// case-insensitive occurrence of end (which starts with '<'). With keep set,
// it returns the bytes before end; at EOF it returns what it has.
func readUntilFold(br *bufio.Reader, end string, keep bool) ([]byte, error) {
	var out []byte
	for {
		chunk, err := br.ReadSlice('<')
		if keep {
			out = append(out, chunk...)
		}
		switch {
		case err == nil:
			next, _ := br.Peek(len(end) - 1)
			if strings.EqualFold(string(next), end[1:]) {
				_, _ = br.Discard(len(end) - 1)
				if keep {
					out = out[:len(out)-1] // drop the '<'
				}
				return out, nil
			}
		case errors.Is(err, bufio.ErrBufferFull):
		default:
			return out, err
		}
	}
}

func isTagNameEnd(c byte) bool {
	return c == '>' || c == '/' || c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// scriptType returns the type attribute of a single <script ...> start tag.
func scriptType(tag []byte) string {
	z := html.NewTokenizer(bytes.NewReader(tag))
	if tt := z.Next(); tt != html.StartTagToken && tt != html.SelfClosingTagToken {
		return ""
	}
	_, hasAttr := z.TagName()
	for hasAttr {
		k, v, more := z.TagAttr()
		if string(k) == "type" {
			return string(v)
		}
		hasAttr = more
	}
	return ""
}
