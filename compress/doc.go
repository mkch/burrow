/*
Package compress provides http compression implementation with "gzip" and
"flate" content-encodings.

A simple use case:
	http.ListenAndServe(":8080", compress.DefaultHandler(http.DefaultServeMux))

The compress.DefaultHandler() function is a shortcut to compress.Handler():

	func Handler(h http.Handler, policy MimePolicy, encFactory EncodingFactory) http.Handler

MIME compression policy is controlled by MimePolicy interface. The DefaultMimePolicy
is the default implementation which allows popular types of http resource to be compressed.

Encoding algorithm selection against "Accept-Encoding" is controlled by
EncodingFactory interface. The DefaultEncodingFactory is the default implementation
which selects the first known encoding.

Implement other content-encodings:

1. Implement your own WriterFactory to create writers of that endocing.

2. Implement a EncodingFactory to return the WriterFactory if this encoding
is accepted in "Accept-Encoding" request header.

3. Call compress.Handler() with your own EncodingFactory.
*/
package compress
