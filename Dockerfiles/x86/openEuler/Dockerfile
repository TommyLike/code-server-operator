FROM ubuntu:18.04

RUN apt-get update && apt-get install -y \
	openssl \
	net-tools \
	git \
	locales \
	sudo \
	dumb-init \
	vim \
	curl \
	wget \
	osc \
	&& rm -rf /var/lib/apt/lists/*

# Install code server operator
RUN curl https://code-server-operator.obs.cn-north-4.myhuaweicloud.com/code-server2.1698-vsc1.41.1-linux-x86_64/code-server \
    -o /usr/local/bin/code-server && \
    chmod +x /usr/local/bin/code-server

RUN locale-gen en_US.UTF-8
# We cannot use update-locale because docker will not use the env variables
# configured in /etc/default/locale so we need to set it manually.
ENV LC_ALL=en_US.UTF-8 \
	SHELL=/bin/bash

RUN adduser --gecos '' --disabled-password coder && \
	echo "coder ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers.d/nopasswd

USER coder
# Create first so these directories will be owned by coder instead of root
# (workdir and mounting appear to both default to root).
RUN mkdir -p /home/coder/project
RUN mkdir -p /home/coder/.local/share/code-server

WORKDIR /home/coder/project

# This ensures we have a volume mounted even if the user forgot to do bind
# mount. So that they do not lose their data if they delete the container.
VOLUME [ "/home/coder/project" ]

# Copy oscrc file into /home/coder/.config/osc/oscrc
COPY oscrc /home/coder/.config/osc/oscrc

EXPOSE 8080

ENTRYPOINT ["dumb-init", "code-server", "--host", "0.0.0.0"]