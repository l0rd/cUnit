# cUnit  [![Circle CI](https://circleci.com/gh/l0rd/docker-unit.png?style=shield)](https://circleci.com/gh/l0rd/docker-unit)
*The Dockerfile Test Framework*

With Dockerfiles you build your environment. With Docker Unit you test it.

[![cUnit in action](https://asciinema.org/a/6dnqlny99u5tfk80yhsqykcfz.png)](https://asciinema.org/a/6dnqlny99u5tfk80yhsqykcfz)

#### Quickstart
This is a `Dockerfile`:
```Dockerfile
FROM debian:jessie

COPY foo.txt /data/

RUN useradd -d /home/mobydock -m -s /bin/bash mobydock
CMD tomcat
```
And this is a `Dockerfile_test` file to test it:
```Dockerfile
@AFTER COPY_FOO
ASSERT_TRUE FILE_EXISTS '/data/foo.txt'

@BEFORE RUN_USERADD 
ASSERT_FALSE USER_EXISTS 'mobydock'

@AFTER USER_ADD
ASSERT_TRUE USER_EXISTS 'mobydock'

@AFTER_RUN
ASSERT_TRUE PROCESS_EXISTS 'java'
```
To test the `Dockerfile`:
```sh
cunit .
```

#### cUnit files syntax

Every test unit in a test file is composed by :

+ a reference to a Dockerfile instruction
+ one or more assertion

For example :
```
@AFTER RUN_USERADD
ASSERT_TRUE USER_EXISTS 'mario'
ASSERT_TRUE FILE_EXISTS '/home/mario/.profile'
```

##### References to Dockerfile instructions
A reference to a Dockerfile instruction has the format
`@[AFTER|BEFORE] <INSTRUCTION>`

Where <INSTRUCTION> should match the prefix of a Dockerfile instruction (spaces are substituded with underscores):
`RUN_USERADD` match `RUN useradd -d /home/mobydock -m -s /bin/bash mobydock`


##### Assertions
Assertions are composed by an assert statement followed by a test condition that can be a shell command or a template:
`[ASSERT_TRUE|ASSERT_FALSE] [<TEST_COMMANDS>|<TEST_TEMPlATES>]`

A test command is a shell boolean conditions. For exemple `-f foo.txt` and `$(whoami)=mario` are both valid test commands. 

Tests templates are some pre-configured boolean conditions. The following tests-templates are available:

 - `FILE_EXISTS foo.txt`
 - `OS_VERSION_MATCH 'ubuntu 14.04 '`
 - `CURRENT_USER_IS 'mario'`
 - `IS_INSTALLED 'vim'`
 - `PROCESS_EXISTS 'httpd'`
 - `IS_LISTENING_ON_PORT 80`
 - `USER_EXISTS 'mario'`
 - `FILE_CONTAINS 'a sentence'`
 - `LOG_CONTAINS 'a sentence'`

##### Includes
Instruction `@INCLUDE` is useful if we need external files to achieve a test. For exemple if the shell script `test_foo.sh` is used to perform a test but is not available inside the Docker image we can include it as follows:

```
# Dockerfile_test
@AFTER CREATE_FOO
@INCLUDE test_foo.sh
ASSERT_TRUE test_foo.sh
```

##### Imports
Some external shell test frameworks can be made available using the `@IMPORT` instruction. Currently supported frameworks are `serverspec` and `bats`.

```
# Dockerfile_test

@AFTER CREATE_FOO
@IMPORT serverspec
@IMPORT bats
ASSERT_TRUE test_foo.sh
```

##### Setup and teardown
`@SETUP` and `@TEARDWON` are two very useful instructions when some instructions (asserts, includes or imports) need to be executed before or after every test block.

```
# Capyfile
@SETUP
@IMPORT bats
```

#### Roadmap

- [x] Dockerfile EPHEMERAL instruction
- [x] ASSERT_TRUE and ASSERT_FALSE support in Capyfile
- [x] Show test results at the end of the process
- [x] Labels in Dockerfile and instructions @BEFORE et @AFTER in Capyfile
- [x] Support test templates
- [ ] Support @INCLUDE
- [ ] Support @SETUP and @TEARDOWN


#### cUnit stands on the shoulders of dockramp

cUnit codebase is heavely based on [dockramp](https://github.com/jlhawn/dockramp) (it's actually a dockramp fork). Therefore the following docker build features are not implemented.

- Handle `.dockerignore`.
- Resolve tag references to digest references using Notary before image pulls.
- Implement various options (via flags) to many Dockerfile instructions.
