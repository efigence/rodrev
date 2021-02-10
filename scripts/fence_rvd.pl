#!/usr/bin/env perl
# puppet managed file, for more info 'puppet-find-resources $filename'
use v5.10;

# template for new commandline scripts
use strict;
use warnings;
use Carp qw(croak cluck carp confess);
use Getopt::Long qw(:config auto_help);
use Pod::Usage;
use Data::Dumper;
use Sys::Syslog;
use Switch;

openlog('fence_rvd', 'ndelay', 'daemon');
syslog($priority, $format, @args);

closelog();
my $cfg = { # default config values go here
    'rv-path' => '/usr/local/bin/rv',
};
my $help;


# https://github.com/ClusterLabs/fence-agents/blob/master/doc/FenceAgentAPI.md

# action - the operation (noted previously) to perform. This is one of the following (case insensitive): on, off, reboot, monitor, list, or status
# ipaddr - for a hostname or IP address
# login - for a username or login name
# passwd - for a password
# passwd_script - if your agent supports storing passwords outside of cluster.conf, this is a script used to retrieve your password (details on how this works will be added later). Generally, this script simply echoes the password to standard output (and is read in by the agent at run-time).
# port - if you have to specify a plug or port (for example, on a network-enabled PDU with 8 ports)
# nodename - if the agent fences by node name, this is the parameter to use (e.g. instead of port). In the event that both nodename and port are specified, the preference is given to port.


my $parse_stdin=0;
if ( !defined($ARGV[0])) {
    $parse_stdin = 1;
}

while(<STDIN>) {
    chomp;
    my ($k, $v) = split(/=/, $_, 2);
    $cfg->{$k} = $v;
}

GetOptions(
    'action=s' => \$cfg->{'action'},
    'ipaddr=s' => \$cfg->{'ipaddr'},
    'login=s' => \$cfg->{'login'},
    'passwd=s' => \$cfg->{'passwd'},
    'passwd-script=s' => \$cfg->{'passwd_script'},
    'port=s' => \$cfg->{'port'},
    'nodename=s' => \$cfg->{'nodename'},
    'help'          => \$help,
) or pod2usage(
    -verbose => 2,  #2 is "full man page" 1 is usage + options ,0/undef is only usage
    -exitval => 1,   #exit with error code if there is something wrong with arguments so anything depending on exit code fails too
);





# some options are required, display short help if user misses them
my $required_opts = [ 'nodename','action' ];
my $missing_opts;
foreach (@$required_opts) {
    if (!defined( $cfg->{$_} ) ) {
        push @$missing_opts, $_
    }
}

if ($help || defined( $missing_opts ) ) {
    my $msg;
    my $verbose = 2;
    if (!$help && defined( $missing_opts ) ) {
        $msg = 'Opts ' . join(', ',@$missing_opts) . " are required!\n";
        $verbose = 1; # only short help on bad arguments
    }
    pod2usage(
        -message => $msg,
        -verbose => $verbose, #exit code doesnt work with verbose > 2, it changes to 1
    );
}
syslog('info', "running [$cfg->{'action'}] on [$cfg->{'nodename'}]");
syslog('info', Dumper $config);


switch(lc($cfg->{'action'})) {
    case /status|monitor/ {
        system($cfg->{'rv-path'}, 'fence', 'status', $cfg->{'nodename'});
        my $exit_code = $? >> 8;
        exit $exit_code;
    }
    case /off|reboot/ {
        system($cfg->{'rv-path'}, 'fence', 'run', $cfg->{'nodename'});
        my $exit_code = $? >> 8;
        exit $exit_code;
    }
    case /on/ {
        # TODO no idea how to verify restart yet
        exit 0;
    }
    case /metadata/ {
        metadata();
        exit 0;
    }
    else {
        print "command [$cfg->{'action'}] not supported"
    }
}

closelog();


sub metadata() {
    print '<?xml version="1.0" ?>
<resource-agent name="fence_rsysrq" shortdesc="Fence agent via rodrev" >
<longdesc>
fence_rvd sends fencing request to rodrev via queue
</longdesc>
<vendor-url>https://efigence.com</vendor-url>
<parameters>
        <parameter name="action" unique="1" required="1">
                <getopt mixed="--action" />
                <content type="string" default="reboot" />
                <shortdesc lang="en">Fencing Action</shortdesc>
        </parameter>
        <parameter name="nodename" unique="1" required="1">
                <getopt mixed="--nodename" />
                <content type="string"  />
                <shortdesc lang="en">name (fqdn or certname) of the node</shortdesc>
        </parameter>
</parameters>
<actions>
    <action name="reboot" />
    <action name="status" />
    <action name="monitor" />
    <action name="metadata" />
</actions>
</resource-agent>
';
}


__END__

=head1 NAME

fence_rvd - fence node via rodrev

=head1 SYNOPSIS

fence_rvd --nodename node.example.com --action status

=head1 DESCRIPTION

Fence node via rodrev

=head1 OPTIONS

parameters can be shortened if unique, like  --add -> -a

=over 4

=item B<--action> on|off|reboot|monitor|list|status

Run fence action

=item B<--nodename> node.example.com

FQDN/certname of the node

=item B<--help>

display full help

=back

=head1 EXAMPLES

=over 4

=item B<fence_rvd>

Pacemaker will passs fencing parameters via STDIN

=item B<fence_rvd --nodename node.example.com --action status>

test if fencing works to given node

=back

=cut
