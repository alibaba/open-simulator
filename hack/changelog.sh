set -o errexit
set -o nounset
set -o pipefail

function join { local IFS="$1"; shift; echo "$*"; }

echo "Generating CHANGELOG markdown\n"

array_name=("feature" "enhancement" "bugfix")
CHANGELOG_PATH=changelogs/unreleased

for i in ${array_name[@]}
do
    TEMPPATH=$CHANGELOG_PATH/$i
    UNRELEASED=$(ls -t ${TEMPPATH})
    for entry in $UNRELEASED
    do
        IFS=$'-' read -ra pruser <<<"$entry"
        contents=$(cat ${TEMPPATH}/${entry})
        pr=${pruser[0]}
        echo "- [$i] ${contents} #${pr}"
    done
done

echo "\nCopy and paste the list above in to the appropriate CHANGELOG file."
echo "Be sure to run: git rm ${CHANGELOG_PATH}/*"
