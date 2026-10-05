module modules

go 1.25.0

require (
	example.com/caps v0.1.0
	example.com/fine v0.1.0
	example.com/link v0.1.0
	example.com/testonly v0.1.0
)

require example.com/words v0.1.0 // indirect

replace (
	example.com/caps => ./third/caps
	example.com/fine => ./third/fine
	example.com/link => ./third/link
	example.com/testonly => ./third/testonly
	example.com/words => ./third/words
)
