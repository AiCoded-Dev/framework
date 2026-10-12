# E-RUN-005: building block not provided here

The app used a building block that the runner where it runs does not provide, such as its file
stores or the sending of its mail. The runner refuses every call to it rather than pretend that the
call worked: nothing was read, written or sent. `aicoded dev` provides every building block.

**Fix:** keep the building block that the error names out of the paths the app takes there
