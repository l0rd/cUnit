FROM tomcat:8.0.28-jre8

RUN useradd -d /home/mario -m -s /bin/bash mario

COPY words /usr/local/tomcat/webapps/

USER mario

RUN bash -c 'echo bar >> /tmp/foo.txt'

RUN apt-get update && apt-get install -y vim

CMD catalina.sh run
